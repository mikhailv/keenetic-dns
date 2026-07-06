// Package parquetdb provides a generic, predicate-based query layer over a parquet file. The DB type is parameterised
// by a Go struct whose `parquet:"..."` tags map to columns; queries are expressed as typed predicates and streamed via
// iter.Seq2.
package parquetdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/parquet-go/parquet-go"
)

// DB is a read-only view of a parquet file with predicate-based queries.
// The type parameter T is the Go struct whose `parquet:"name,..."` tags map fields to columns.
type DB[T any] struct {
	logger *slog.Logger
	file   *os.File
	pf     *parquet.File
	colIdx map[string]int // column name -> column-chunk index within a row group
	fields map[string]int // column name -> T struct field index (top-level)

	sortedMu  sync.Mutex
	sortedAsc map[string]bool // column name -> whole-file ascending? (lazy)
}

// Open opens path as a typed parquet DB. The caller must Close it when done.
func Open[T any](path string, logger *slog.Logger) (*DB[T], error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening parquet file: %w", err)
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat parquet file: %w", err)
	}
	pf, err := parquet.OpenFile(f, stat.Size())
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("parsing parquet file: %w", err)
	}

	db := &DB[T]{
		logger: logger,
		file:   f,
		pf:     pf,
		colIdx: map[string]int{},
		fields: map[string]int{},
	}
	for i, col := range pf.Root().Columns() {
		db.colIdx[col.Name()] = i
	}

	var zero T
	rt := reflect.TypeOf(zero)
	if rt.Kind() != reflect.Struct {
		_ = f.Close()
		return nil, fmt.Errorf("type parameter must be a struct, got %s", rt.Kind())
	}
	for i := range rt.NumField() {
		sf := rt.Field(i)
		tag := sf.Tag.Get("parquet")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = sf.Name
		}
		db.fields[name] = i
	}

	return db, nil
}

// Close releases the underlying file handle.
func (db *DB[T]) Close() error {
	return db.file.Close()
}

// HasColumn reports whether the file has a column with the given name.
func (db *DB[T]) HasColumn(name string) bool {
	_, ok := db.colIdx[name]
	return ok
}

// Query scans the file and yields rows that satisfy every predicate (AND).
// Iteration honors ctx cancellation and stops if the consumer breaks.
//
// Calling Query with no predicates yields every row in the file.
func (db *DB[T]) Query(ctx context.Context, preds ...Predicate) iter.Seq2[T, error] {
	if len(preds) == 1 && preds[0].op == opRangeContainsInt64 && db.isAscending(preds[0].field) {
		return db.queryRangeSorted(ctx, preds[0])
	}
	return db.queryScan(ctx, preds)
}

// predState bundles a predicate with the resolved struct-field and column indices for each of its columns.
type predState struct {
	pred      Predicate
	fieldIdx  int
	fieldIdx2 int // -1 if pred has no secondary field
	colIdx    int // -1 if column is absent from the file
	colIdx2   int // -1 if absent / not used
}

func (db *DB[T]) buildStates(preds []Predicate) ([]predState, error) {
	states := make([]predState, len(preds))
	for i, p := range preds {
		fi, ok := db.fields[p.field]
		if !ok {
			return nil, fmt.Errorf("unknown field %q for row type %T", p.field, *new(T))
		}
		states[i] = predState{
			pred:      p,
			fieldIdx:  fi,
			fieldIdx2: -1,
			colIdx:    db.lookupColIdx(p.field),
			colIdx2:   -1,
		}
		if p.field2 != "" {
			fi2, ok := db.fields[p.field2]
			if !ok {
				return nil, fmt.Errorf("unknown field %q for row type %T", p.field2, *new(T))
			}
			states[i].fieldIdx2 = fi2
			states[i].colIdx2 = db.lookupColIdx(p.field2)
		}
	}
	return states, nil
}

func (db *DB[T]) lookupColIdx(name string) int {
	if i, ok := db.colIdx[name]; ok {
		return i
	}
	return -1
}

func (db *DB[T]) queryScan(ctx context.Context, preds []Predicate) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		states, err := db.buildStates(preds)
		if err != nil {
			yield(zero, err)
			return
		}
		project := canProject(states)
		for _, rg := range db.pf.RowGroups() {
			if err := ctx.Err(); err != nil {
				yield(zero, err)
				return
			}
			skip, err := shouldSkipRowGroup(rg, states)
			if err != nil {
				if !yield(zero, err) {
					return
				}
				continue
			}
			if skip {
				continue
			}
			scan := scanRowGroup[T]
			if project {
				scan = scanRowGroupProjected[T]
			}
			if !scan(ctx, rg, states, yield) {
				return
			}
		}
	}
}

// canProject reports whether the predicate set can be evaluated with a projection scan: every column it reads must be
// present in the file. When any column is absent, the full-decode path must be used so the missing column evaluates
// against the struct field's zero value. Zero predicates cannot project (there is nothing to read; the full scan yields
// every row).
func canProject(states []predState) bool {
	if len(states) == 0 {
		return false
	}
	for _, s := range states {
		if s.colIdx < 0 {
			return false
		}
		if s.fieldIdx2 >= 0 && s.colIdx2 < 0 {
			return false
		}
	}
	return true
}

// shouldSkipRowGroup returns true if no row in rg can satisfy every predicate, based on column-index page bounds.
// Predicates referencing columns absent from the file are ignored at this stage and evaluated row-by-row.
func shouldSkipRowGroup(rg parquet.RowGroup, states []predState) (bool, error) {
	chunks := rg.ColumnChunks()
	for _, s := range states {
		skip, err := columnPrunesRowGroup(chunks, s.colIdx, s.pred, s.pred.field)
		if err != nil || skip {
			return skip, err
		}
		skip, err = columnPrunesRowGroup(chunks, s.colIdx2, s.pred, s.pred.field2)
		if err != nil || skip {
			return skip, err
		}
	}
	return false, nil
}

// columnPrunesRowGroup reports whether column colIdx's page bounds prove no row can satisfy p. A negative colIdx (column
// absent from the file, or an unused range end) never prunes.
func columnPrunesRowGroup(chunks []parquet.ColumnChunk, colIdx int, p Predicate, field string) (bool, error) {
	if colIdx < 0 {
		return false, nil
	}
	ci, err := chunks[colIdx].ColumnIndex()
	if err != nil {
		return false, fmt.Errorf("reading column index for %q: %w", field, err)
	}
	return !anyPageCanMatch(ci, p, field, chunks[colIdx].Type().Kind()), nil
}

func anyPageCanMatch(ci parquet.ColumnIndex, p Predicate, field string, kind parquet.Kind) bool {
	// A null decodes to the column's zero value; page min/max bounds exclude nulls. So a predicate that matches the
	// zero value can be satisfied by a null even when the non-null bounds do not bracket the query value.
	nullMatches := p.matchesZeroValue(kind)
	for pi := range ci.NumPages() {
		if p.canMatchPage(field, ci.MinValue(pi), ci.MaxValue(pi)) {
			return true
		}
		if nullMatches && (ci.NullPage(pi) || ci.NullCount(pi) > 0) {
			return true
		}
	}
	return false
}

func scanRowGroup[T any](ctx context.Context, rg parquet.RowGroup, states []predState, yield func(T, error) bool) bool {
	reader := parquet.NewGenericRowGroupReader[T](rg)
	defer reader.Close()

	var zero T
	var buf [64]T
	for {
		if err := ctx.Err(); err != nil {
			yield(zero, err)
			return false
		}
		n, err := reader.Read(buf[:])
		for i := range n {
			if !matchAll(&buf[i], states) {
				continue
			}
			if !yield(buf[i], nil) {
				return false
			}
		}
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			yield(zero, err)
			return false
		}
	}
}

// projState pairs a predicate with the raw column values read for its column(s) in one row group. vals holds the
// primary column (row-aligned by index); vals2 holds the range end column, or nil for non-range predicates.
type projState struct {
	pred  Predicate
	vals  []parquet.Value
	kind  parquet.Kind
	vals2 []parquet.Value
}

// scanRowGroupProjected evaluates the predicates by reading only their columns, collecting the offsets of matching
// rows, then materializing just those rows. This avoids decoding every column of every row, which is the dominant cost
// when scanning an unsorted column (e.g. city_geoname_id) that min/max pruning cannot narrow. Rows are yielded in row
// order, identical to scanRowGroup.
func scanRowGroupProjected[T any](
	ctx context.Context,
	rg parquet.RowGroup,
	states []predState,
	yield func(T, error) bool,
) bool {
	var zero T
	numRows := int(rg.NumRows())
	chunks := rg.ColumnChunks()

	// Read each referenced column once, even when several predicates share it.
	valCache := map[int][]parquet.Value{}
	readCol := func(ci int) ([]parquet.Value, error) {
		if v, ok := valCache[ci]; ok {
			return v, nil
		}
		v, err := readColumnValues(chunks[ci], numRows)
		if err != nil {
			return nil, err
		}
		valCache[ci] = v
		return v, nil
	}

	ps := make([]projState, len(states))
	for i, s := range states {
		vals, err := readCol(s.colIdx)
		if err != nil {
			yield(zero, fmt.Errorf("reading column %q: %w", s.pred.field, err))
			return false
		}
		ps[i] = projState{pred: s.pred, vals: vals, kind: chunks[s.colIdx].Type().Kind()}
		if s.colIdx2 >= 0 {
			vals2, err := readCol(s.colIdx2)
			if err != nil {
				yield(zero, fmt.Errorf("reading column %q: %w", s.pred.field2, err))
				return false
			}
			ps[i].vals2 = vals2
		}
	}

	var matches []int
	for i := range numRows {
		if matchRowProjected(ps, i) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		return true
	}

	reader := parquet.NewGenericRowGroupReader[T](rg)
	defer reader.Close()
	var buf [1]T
	for _, m := range matches {
		if err := ctx.Err(); err != nil {
			yield(zero, err)
			return false
		}
		if err := reader.SeekToRow(int64(m)); err != nil {
			yield(zero, fmt.Errorf("seeking to row %d: %w", m, err))
			return false
		}
		n, err := reader.Read(buf[:])
		if n == 1 {
			if !yield(buf[0], nil) {
				return false
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			yield(zero, err)
			return false
		}
	}
	return true
}

func matchRowProjected(ps []projState, i int) bool {
	for _, s := range ps {
		if s.pred.op == opRangeContainsInt64 {
			if !s.pred.matchRangeInt64(int64OrZero(s.vals[i]), int64OrZero(s.vals2[i])) {
				return false
			}
			continue
		}
		if !s.pred.matchProjectedValue(s.vals[i], s.kind) {
			return false
		}
	}
	return true
}

func int64OrZero(v parquet.Value) int64 {
	if v.IsNull() {
		return 0
	}
	return v.Int64()
}

// readColumnValues reads all values of a single column chunk into a row-aligned slice (one value per row, nulls
// included). Values are cloned so byte-array payloads stay valid after their backing page is released.
func readColumnValues(chunk parquet.ColumnChunk, numRows int) ([]parquet.Value, error) {
	out := make([]parquet.Value, 0, numRows)
	pages := chunk.Pages()
	defer func() { _ = pages.Close() }()
	buf := make([]parquet.Value, 1024)
	for {
		pg, err := pages.ReadPage()
		if pg != nil {
			vr := pg.Values()
			for {
				n, verr := vr.ReadValues(buf)
				for i := range n {
					out = append(out, buf[i].Clone())
				}
				if verr != nil {
					if errors.Is(verr, io.EOF) {
						break
					}
					parquet.Release(pg)
					return nil, verr
				}
			}
			parquet.Release(pg)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}
	return out, nil
}

func matchAll[T any](row *T, states []predState) bool {
	if len(states) == 0 {
		return true
	}
	rv := reflect.ValueOf(row).Elem()
	for _, s := range states {
		if !matchPredicate(s, rv) {
			return false
		}
	}
	return true
}

func matchPredicate(s predState, rv reflect.Value) bool {
	if s.pred.op == opRangeContainsInt64 {
		return s.pred.matchRangeInt64(
			rv.Field(s.fieldIdx).Int(),
			rv.Field(s.fieldIdx2).Int(),
		)
	}
	return matchValue(s.pred, rv.Field(s.fieldIdx))
}

func matchValue(p Predicate, v reflect.Value) bool {
	switch v.Kind() { //nolint:exhaustive // unsupported kinds fall through to false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if p.isStringOp() {
			return p.matchString(strconv.FormatInt(v.Int(), 10))
		}
		return p.matchInt64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if p.isStringOp() {
			return p.matchString(strconv.FormatUint(v.Uint(), 10))
		}
		return p.matchInt64(int64(v.Uint()))
	case reflect.String:
		return p.matchString(v.String())
	default:
		return false
	}
}

// queryRangeSorted is the fast path for RangeContainsInt64 against an ascending start column. It binary-searches across
// row groups, narrows to a page within the candidate row group, then linearly scans rows until either a containing
// range is found or start > target proves the answer is absent.
//
// Correctness assumes ranges in the data are non-overlapping, which is true for IP geolocation tables and similar.
func (db *DB[T]) queryRangeSorted(ctx context.Context, p Predicate) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T

		startFieldIdx, ok := db.fields[p.field]
		if !ok {
			yield(zero, fmt.Errorf("unknown field %q for row type %T", p.field, zero))
			return
		}
		endFieldIdx, ok := db.fields[p.field2]
		if !ok {
			yield(zero, fmt.Errorf("unknown field %q for row type %T", p.field2, zero))
			return
		}
		startColIdx, ok := db.colIdx[p.field]
		if !ok {
			yield(zero, fmt.Errorf("column %q not present in file", p.field))
			return
		}

		rg, firstRow, err := locateRangeCandidate(db.pf.RowGroups(), startColIdx, p.intVal)
		if err != nil {
			yield(zero, err)
			return
		}
		if rg == nil {
			return
		}
		scanForRange(ctx, rg, firstRow, p.intVal, startFieldIdx, endFieldIdx, yield)
	}
}

// locateRangeCandidate binary-searches across row groups to find the row that could contain target, and returns the row
// group and the first row offset within it to start scanning from. Returns (nil, 0, nil) if target is before the first
// range.
func locateRangeCandidate(
	rowGroups []parquet.RowGroup,
	startColIdx int,
	target int64,
) (parquet.RowGroup, int64, error) {
	rgIdx := sort.Search(len(rowGroups), func(i int) bool {
		ci, err := rowGroups[i].ColumnChunks()[startColIdx].ColumnIndex()
		if err != nil {
			return false
		}
		return ci.MinValue(0).Int64() > target
	}) - 1
	if rgIdx < 0 {
		return nil, 0, nil
	}
	rg := rowGroups[rgIdx]
	startChunk := rg.ColumnChunks()[startColIdx]
	startCI, err := startChunk.ColumnIndex()
	if err != nil {
		return nil, 0, fmt.Errorf("reading column index: %w", err)
	}

	pageIdx := parquet.Search(startCI, parquet.Int64Value(target), startChunk.Type())
	if pageIdx >= startCI.NumPages() {
		pageIdx = startCI.NumPages() - 1
	}
	// The matching row has start <= target, which may sit one page back.
	if pageIdx > 0 && startCI.MinValue(pageIdx).Int64() > target {
		pageIdx--
	}

	offsetIdx, err := startChunk.OffsetIndex()
	if err != nil {
		return nil, 0, fmt.Errorf("reading offset index: %w", err)
	}
	return rg, offsetIdx.FirstRowIndex(pageIdx), nil
}

func scanForRange[T any](
	ctx context.Context,
	rg parquet.RowGroup,
	firstRow, target int64,
	startFieldIdx, endFieldIdx int,
	yield func(T, error) bool,
) {
	var zero T
	reader := parquet.NewGenericRowGroupReader[T](rg)
	defer reader.Close()
	if firstRow > 0 {
		if err := reader.SeekToRow(firstRow); err != nil {
			yield(zero, fmt.Errorf("seeking to row %d: %w", firstRow, err))
			return
		}
	}

	var buf [16]T
	for {
		if err := ctx.Err(); err != nil {
			yield(zero, err)
			return
		}
		n, err := reader.Read(buf[:])
		for i := range n {
			rv := reflect.ValueOf(&buf[i]).Elem()
			start := rv.Field(startFieldIdx).Int()
			if start > target {
				return
			}
			if rv.Field(endFieldIdx).Int() >= target {
				yield(buf[i], nil)
				return
			}
		}
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			yield(zero, err)
			return
		}
	}
}

// isAscending reports whether the given column is sorted ascending across the whole file.
// The result is cached after the first computation.
func (db *DB[T]) isAscending(name string) bool {
	db.sortedMu.Lock()
	defer db.sortedMu.Unlock()
	if db.sortedAsc == nil {
		db.sortedAsc = map[string]bool{}
	}
	if v, ok := db.sortedAsc[name]; ok {
		return v
	}
	result := db.computeAscending(name)
	db.sortedAsc[name] = result
	if result {
		db.logger.Info("column is sorted ascending, enabling binary-search lookup", "column", name)
	} else {
		db.logger.Info("column is not sorted ascending, falling back to scan", "column", name)
	}
	return result
}

func (db *DB[T]) computeAscending(name string) bool {
	idx, ok := db.colIdx[name]
	if !ok {
		return false
	}
	rowGroups := db.pf.RowGroups()
	if len(rowGroups) == 0 {
		return false
	}

	var prevMax int64
	havePrev := false
	for i, rg := range rowGroups {
		// Respect SortingColumns metadata when present.
		if sortCols := rg.SortingColumns(); len(sortCols) > 0 {
			found := false
			for _, sc := range sortCols {
				if len(sc.Path()) == 1 && sc.Path()[0] == name && !sc.Descending() {
					found = true
					break
				}
			}
			if !found {
				db.logger.Warn("row group has sorting columns but target is not ascending",
					"column", name, "rowGroup", i)
				return false
			}
		}

		ci, err := rg.ColumnChunks()[idx].ColumnIndex()
		if err != nil {
			db.logger.Warn("cannot read column index",
				"column", name, "rowGroup", i, "err", err)
			return false
		}
		if ci.NumPages() > 1 && !ci.IsAscending() {
			db.logger.Warn("column index not ascending within row group",
				"column", name, "rowGroup", i)
			return false
		}

		curMin := ci.MinValue(0).Int64()
		if havePrev && curMin <= prevMax {
			db.logger.Warn("column not sorted across row groups",
				"column", name, "rowGroup", i, "min", curMin, "prevMax", prevMax)
			return false
		}
		prevMax = ci.MaxValue(ci.NumPages() - 1).Int64()
		havePrev = true
	}
	return true
}
