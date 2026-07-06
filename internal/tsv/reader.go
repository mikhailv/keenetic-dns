package tsv

import (
	"bufio"
	"errors"
	"io"
	"iter"
	"reflect"
	"strings"
)

// defaultMaxLineSize is the bufio.Scanner default token size; lines longer
// than this fail with bufio.ErrTooLong unless overridden via WithMaxLineSize.
const defaultMaxLineSize = 64 * 1024

// ReaderOption configures a Reader at construction time.
type ReaderOption func(*readerOptions)

type readerOptions struct {
	maxLineSize int
}

// WithMaxLineSize sets the maximum size of a single TSV line, in bytes.
// Lines longer than this fail with bufio.ErrTooLong. The default is 64 KiB.
func WithMaxLineSize(n int) ReaderOption {
	return func(o *readerOptions) {
		o.maxLineSize = n
	}
}

// Reader reads TSV data from an io.Reader and unmarshalls it into struct values.
// The generic type parameter T must be a struct type with exported fields
// tagged with `tsv` tags for column names.
//
// Example:
//
//	type Person struct {
//	    Name string `tsv:"name"`
//	    Age  int    `tsv:"age,optional"`
//	}
//
//	r := tsv.NewReader[Person](file)
//	row, err := r.Read()
//
// A Reader is not safe for concurrent use.
type Reader[T any] struct {
	scanner  *bufio.Scanner
	typeInfo *typeInfo
	colMap   map[int]int // field index -> column index in header; nil until header is parsed
	value    T           // Reused buffer for row values
	row      int         // Current row number (1-based, header is line 1)
	err      error       // Sticky error; once set, every Read returns it
}

// NewReader creates a new Reader for type T that reads from r.
func NewReader[T any](r io.Reader, opts ...ReaderOption) *Reader[T] {
	o := readerOptions{maxLineSize: defaultMaxLineSize}
	for _, opt := range opts {
		opt(&o)
	}
	scanner := bufio.NewScanner(r)
	initBuf := min(o.maxLineSize, defaultMaxLineSize)
	scanner.Buffer(make([]byte, 0, initBuf), o.maxLineSize)
	return &Reader[T]{
		scanner: scanner,
	}
}

func (r *Reader[T]) init() error {
	var err error
	r.typeInfo, err = getTypeInfo[T]()
	return err
}

// Read reads the next row from the TSV data.
// Returns io.EOF when there are no more rows.
// The struct fields are mapped from TSV columns using the tsv tag.
// Columns in the TSV that don't match any struct field are ignored.
// Missing optional fields are set to their zero value.
// Missing required fields return MissingColumnValueError.
//
// If header parsing fails (or the input is empty), the same error is returned
// from every subsequent call — the reader does not attempt recovery.
func (r *Reader[T]) Read() (T, error) {
	if r.err != nil {
		return zeroVal[T](), r.err
	}
	if r.colMap == nil {
		r.row++
		if err := r.init(); err != nil {
			r.err = err
			return zeroVal[T](), err
		}
		line, err := r.scanNextLine()
		if err != nil {
			r.err = err
			return zeroVal[T](), err
		}
		if err := r.parseHeader(line); err != nil {
			r.err = err
			return zeroVal[T](), err
		}
	}

	line, err := r.scanNextLine()
	if err != nil {
		// Scanner errors (including io.EOF and bufio.ErrTooLong) leave the
		// scanner unable to produce more rows, so make them sticky.
		r.err = err
		return zeroVal[T](), err
	}

	r.row++

	rowVals := strings.Split(line, "\t")

	// Reset the buffer so optional/short-row fields don't leak the previous
	// row's value into the next one.
	var zero T
	r.value = zero
	result := reflect.ValueOf(&r.value).Elem()

	for i, f := range r.typeInfo.fields {
		colIdx, ok := r.colMap[i]
		if !ok {
			continue
		}

		if colIdx >= len(rowVals) {
			if !f.optional {
				return zeroVal[T](), MissingColumnValueError{f.name, r.row}
			}
			continue
		}

		val := rowVals[colIdx]

		fieldVal, err := f.parse(val)
		if err != nil {
			return zeroVal[T](), ValueParseError{f.name, val, r.row, err}
		}

		result.FieldByIndex(f.index).Set(fieldVal)
	}

	return result.Interface().(T), nil //nolint:errcheck // expected type
}

// scanNextLine reads the next line from the scanner.
func (r *Reader[T]) scanNextLine() (string, error) {
	if r.scanner.Scan() {
		return r.scanner.Text(), nil
	}
	if err := r.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// parseHeader parses the TSV header line and builds a column mapping.
func (r *Reader[T]) parseHeader(headerLine string) error {
	if headerLine == "" {
		return ErrNoColumns
	}

	headerCols := strings.Split(headerLine, "\t")
	for i := range headerCols {
		headerCols[i] = strings.TrimSpace(headerCols[i])
		if headerCols[i] == "" {
			return EmptyColumnNameError{Index: i}
		}
	}

	headerIndex := map[string]int{}
	for idx, name := range headerCols {
		if existing, dup := headerIndex[name]; dup {
			return DuplicateColumnError{Column: name, FirstIndex: existing, DuplicateIndex: idx}
		}
		headerIndex[name] = idx
	}

	r.colMap = map[int]int{}
	for i, f := range r.typeInfo.fields {
		if idx, ok := headerIndex[f.name]; ok {
			r.colMap[i] = idx
		} else if !f.optional {
			return MissingColumnError{Column: f.name}
		}
	}

	return nil
}

// Iterator returns an iterator over all rows in the TSV data.
//
// Errors are classified as recoverable or sticky:
//   - Recoverable errors (ValueParseError, MissingColumnValueError) describe a
//     single bad row; the scanner has advanced past it, so iteration continues
//     with the next row unless the caller stops it by returning false from the
//     yield function.
//   - Sticky errors (header errors, scanner I/O errors such as bufio.ErrTooLong)
//     leave the Reader unable to make progress; the iterator yields the error
//     once and then stops.
//
// io.EOF is never yielded — it terminates iteration silently.
//
// Example:
//
//	for row, err := range r.Iterator() {
//		if err != nil {
//			// inspect / log; break here to stop on the first error
//			continue
//		}
//		// use row
//	}
func (r *Reader[T]) Iterator() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for {
			row, err := r.Read()
			if errors.Is(err, io.EOF) {
				return
			}
			if !yield(row, err) {
				return
			}
			if r.err != nil {
				return
			}
		}
	}
}
