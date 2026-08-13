package parquetdb

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
)

type testRow struct {
	Start int64  `parquet:"start_int"`
	End   int64  `parquet:"end_int"`
	City  int64  `parquet:"city_id,optional"`
	Name  string `parquet:"name,optional"`
}

func writeTestParquet[T any](tb testing.TB, rows []T, rowGroupSize int) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "test.parquet")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatalf("create: %v", err)
	}
	defer f.Close()
	w := parquet.NewGenericWriter[T](f, parquet.MaxRowsPerRowGroup(int64(rowGroupSize)))
	if _, err := w.Write(rows); err != nil {
		tb.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		tb.Fatalf("close writer: %v", err)
	}
	return path
}

func collect[T any](tb testing.TB, db *DB[T], preds ...Predicate) []T {
	tb.Helper()
	var out []T
	for row, err := range db.Query(tb.Context(), preds...) {
		if err != nil {
			tb.Fatalf("query error: %v", err)
		}
		out = append(out, row)
	}
	return out
}

// makeTestRows builds n rows with sorted, non-overlapping [Start,End] ranges and repeating City/Name values.
func makeTestRows(n int) []testRow {
	rows := make([]testRow, n)
	for i := range n {
		start := int64(i) * 10
		rows[i] = testRow{
			Start: start,
			End:   start + 9,
			City:  int64(i % 100), // duplicates: each city id appears n/100 times
			Name:  fmt.Sprintf("city-%02d", i%50),
		}
	}
	return rows
}

func openTestDB(tb testing.TB, path string) *DB[testRow] {
	tb.Helper()
	db, err := Open[testRow](path, slog.New(slog.DiscardHandler))
	if err != nil {
		tb.Fatalf("open: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	return db
}

func TestProjectionScanCorrectness(t *testing.T) {
	rows := makeTestRows(2500)
	// Small row groups so the scan crosses many groups and exercises min/max pruning.
	db := openTestDB(t, writeTestParquet(t, rows, 100))

	tests := []struct {
		name string
		pred []Predicate
		want func(testRow) bool
	}{
		{
			name: "eq int on unsorted column",
			pred: []Predicate{EqInt64("city_id", 42)},
			want: func(r testRow) bool { return r.City == 42 },
		},
		{
			name: "eq string via stringified int query (Wildcard degrade)",
			pred: []Predicate{Wildcard("city_id", "7")},
			want: func(r testRow) bool { return r.City == 7 },
		},
		{
			name: "eq string on string column",
			pred: []Predicate{EqString("name", "city-13")},
			want: func(r testRow) bool { return r.Name == "city-13" },
		},
		{
			name: "wildcard on string column",
			pred: []Predicate{Wildcard("name", "city-1?")},
			want: func(r testRow) bool { return len(r.Name) == 7 && r.Name[:6] == "city-1" },
		},
		{
			name: "AND of two columns",
			pred: []Predicate{EqInt64("city_id", 20), Wildcard("name", "city-2*")},
			want: func(r testRow) bool { return r.City == 20 && r.Name[:6] == "city-2" },
		},
		{
			name: "range + eq forces projected range scan",
			pred: []Predicate{RangeContainsInt64("start_int", "end_int", 12345), GEInt64("city_id", 0)},
			want: func(r testRow) bool { return r.Start <= 12345 && 12345 <= r.End && r.City >= 0 },
		},
		{
			name: "no match",
			pred: []Predicate{EqInt64("city_id", 9999)},
			want: func(testRow) bool { return false },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := collect(t, db, tc.pred...)
			var want []testRow
			for _, r := range rows {
				if tc.want(r) {
					want = append(want, r)
				}
			}
			assertRowsEqual(t, want, got)
		})
	}
}

// TestProjectionScanMatchesFullDecode guards parity: the projected result must equal the reflection-based full-decode
// result for the same predicates, including row order.
func TestProjectionScanMatchesFullDecode(t *testing.T) {
	rows := makeTestRows(1500)
	db := openTestDB(t, writeTestParquet(t, rows, 128))

	preds := []Predicate{EqInt64("city_id", 55)}
	states, err := db.buildStates(preds)
	if err != nil {
		t.Fatalf("buildStates: %v", err)
	}

	projected := runScan(t, db, states, scanRowGroupProjected[testRow])
	full := runScan(t, db, states, scanRowGroup[testRow])
	assertRowsEqual(t, full, projected)
	if len(projected) == 0 {
		t.Fatal("expected some matches")
	}
}

// TestProjectionNullHandling verifies a null optional column is treated as the Go zero value, matching the full-decode
// path (which decodes a missing optional column to the struct field's zero value).
func TestProjectionNullHandling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nulls.parquet")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	schema := parquet.SchemaOf(testRow{})
	w := parquet.NewGenericWriter[map[string]any](f, schema, parquet.MaxRowsPerRowGroup(4))
	recs := []map[string]any{
		{"start_int": int64(0), "end_int": int64(9), "city_id": int64(5), "name": "a"},
		{"start_int": int64(10), "end_int": int64(19)},                      // city_id + name null
		{"start_int": int64(20), "end_int": int64(29), "city_id": int64(0)}, // explicit zero, name null
		{"start_int": int64(30), "end_int": int64(39), "name": ""},          // explicit empty, city null
	}
	if _, err := w.Write(recs); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	db := openTestDB(t, path)

	// city_id == 0 must match the three rows where city is null or explicitly 0 (rows 1,2,3 -> null,0,null).
	got := collect(t, db, EqInt64("city_id", 0))
	if len(got) != 3 {
		t.Fatalf("EqInt64(city_id,0): got %d rows, want 3 (null and zero treated equal): %+v", len(got), got)
	}
	// name == "" must match the three rows where name is null (rows 1,2) or explicitly empty (row 3).
	gotName := collect(t, db, EqString("name", ""))
	if len(gotName) != 3 {
		t.Fatalf("EqString(name,\"\"): got %d rows, want 3: %+v", len(gotName), gotName)
	}
}

type scanFunc[T any] func(context.Context, parquet.RowGroup, []predState, func(T, error) bool) bool

func runScan[T any](tb testing.TB, db *DB[T], states []predState, scan scanFunc[T]) []T {
	tb.Helper()
	var out []T
	for _, rg := range db.pf.RowGroups() {
		scan(tb.Context(), rg, states, func(row T, err error) bool {
			if err != nil {
				tb.Fatalf("scan error: %v", err)
			}
			out = append(out, row)
			return true
		})
	}
	return out
}

func assertRowsEqual[T any](tb testing.TB, want, got []T) {
	tb.Helper()
	if len(want) != len(got) {
		tb.Fatalf("row count: want %d, got %d", len(want), len(got))
	}
	for i := range want {
		if fmt.Sprintf("%v", want[i]) != fmt.Sprintf("%v", got[i]) {
			tb.Fatalf("row %d: want %v, got %v", i, want[i], got[i])
		}
	}
}

// BenchmarkFieldScan measures an equality query on an unsorted column — the pathological case that motivated the
// projection scan. Compare against the pre-projection full-decode cost by running with -tags to swap paths, or simply
// observe the absolute number.
func BenchmarkFieldScan(b *testing.B) {
	rows := makeTestRows(200000)
	db := openTestDB(b, writeTestParquet(b, rows, 1000)) // 200 tiny row groups, like the real geo DB

	b.ResetTimer()
	for range b.N {
		n := 0
		for _, err := range db.Query(b.Context(), EqInt64("city_id", 42)) {
			if err != nil {
				b.Fatal(err)
			}
			n++
		}
		if n == 0 {
			b.Fatal("no matches")
		}
	}
}
