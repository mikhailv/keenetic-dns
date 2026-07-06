package tsv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReader_Next(t *testing.T) {
	data := "name\tage\temail\nJohn\t25\tjohn@example.com"
	r := NewReader[SimpleRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row.Name)
	assert.Equal(t, 25, row.Age)
	assert.Equal(t, "john@example.com", row.Email)
}

func TestReader_EOF(t *testing.T) {
	r := NewReader[SimpleRow](strings.NewReader(""))
	_, err := r.Read()
	assert.Equal(t, io.EOF, err)
}

func TestReader_EmptyFile(t *testing.T) {
	type EmptyRow struct {
		Name string `tsv:"name,optional"`
	}
	r := NewReader[EmptyRow](strings.NewReader("\n"))
	_, err := r.Read()
	assert.Equal(t, ErrNoColumns, err)
}

func TestReader_MultipleRows(t *testing.T) {
	data := "name\tage\temail\nJohn\t25\tjohn@example.com\nJane\t30\tjane@example.com"
	r := NewReader[SimpleRow](strings.NewReader(data))

	row1, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row1.Name)
	assert.Equal(t, 25, row1.Age)

	row2, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "Jane", row2.Name)
	assert.Equal(t, 30, row2.Age)

	_, err = r.Read()
	assert.Equal(t, io.EOF, err)
}

func TestReader_ExtraColumns(t *testing.T) {
	data := "name\tage\textra1\textra2\temail\nJohn\t25\tfoo\tbar\tjohn@example.com"
	r := NewReader[SimpleRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row.Name)
	assert.Equal(t, 25, row.Age)
	assert.Equal(t, "john@example.com", row.Email)
}

func TestReader_DifferentColumnOrder(t *testing.T) {
	data := "email\tname\tage\njohn@example.com\tJohn\t25"
	r := NewReader[SimpleRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row.Name)
	assert.Equal(t, 25, row.Age)
	assert.Equal(t, "john@example.com", row.Email)
}

func TestReader_NoTagUsesFieldName(t *testing.T) {
	data := "user_name\tuser_age\tEmail\nJohn\t25\ttest@example.com"
	r := NewReader[RowWithMixedTags](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row.Name)
	assert.Equal(t, 25, row.Age)
	assert.Equal(t, "test@example.com", row.Email)
}

func TestReader_EmbeddedStruct(t *testing.T) {
	data := "created\tname\n2024-01-01\tJohn"
	r := NewReader[RowWithEmbedded](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "2024-01-01", row.Created)
	assert.Equal(t, "John", row.Name)
}

func TestReader_ParsesAllTypes(t *testing.T) {
	data := "str\tint\tint8\tint16\tint32\tint64\tuint\tuint8\tuint16\tuint32\tuint64\tfloat32\tfloat64\tbool\n" +
		"hello\t1\t2\t3\t4\t5\t6\t7\t8\t9\t10\t3.14\t6.28\ttrue"
	r := NewReader[RowWithAllTypes](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "hello", row.String)
	assert.Equal(t, 1, row.Int)
	assert.Equal(t, int8(2), row.Int8)
	assert.Equal(t, int16(3), row.Int16)
	assert.Equal(t, int32(4), row.Int32)
	assert.Equal(t, int64(5), row.Int64)
	assert.Equal(t, uint(6), row.Uint)
	assert.Equal(t, uint8(7), row.Uint8)
	assert.Equal(t, uint16(8), row.Uint16)
	assert.Equal(t, uint32(9), row.Uint32)
	assert.Equal(t, uint64(10), row.Uint64)
	assert.Equal(t, float32(3.14), row.Float32)
	assert.Equal(t, 6.28, row.Float64)
	assert.True(t, row.Bool)
}

func TestReader_BoolVariants(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"true lowercase", "true"},
		{"TRUE uppercase", "TRUE"},
		{"one", "1"},
		{"false lowercase", "false"},
		{"FALSE uppercase", "FALSE"},
		{"zero", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type BoolRow struct {
				Bool bool `tsv:"bool"`
			}
			data := "bool\n" + tt.value
			r := NewReader[BoolRow](strings.NewReader(data))

			row, err := r.Read()
			assert.NoError(t, err)
			assert.Equal(t, tt.value == "true" || tt.value == "TRUE" || tt.value == "1", row.Bool)
		})
	}
}

func TestReader_ZeroValues(t *testing.T) {
	data := "str\tint\tint8\tint16\tint32\tint64\tuint\tuint8\tuint16\tuint32\tuint64\tfloat32\tfloat64\tbool\n" +
		"\t0\t0\t0\t0\t0\t0\t0\t0\t0\t0\t0\t0\tfalse"
	r := NewReader[RowWithAllTypes](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "", row.String)
	assert.Equal(t, 0, row.Int)
	assert.Equal(t, int8(0), row.Int8)
	assert.Equal(t, int16(0), row.Int16)
	assert.Equal(t, int32(0), row.Int32)
	assert.Equal(t, int64(0), row.Int64)
	assert.Equal(t, uint(0), row.Uint)
	assert.Equal(t, uint8(0), row.Uint8)
	assert.Equal(t, uint16(0), row.Uint16)
	assert.Equal(t, uint32(0), row.Uint32)
	assert.Equal(t, uint64(0), row.Uint64)
	assert.Equal(t, float32(0), row.Float32)
	assert.Equal(t, 0.0, row.Float64)
	assert.False(t, row.Bool)
}

func TestReader_NegativeNumbers(t *testing.T) {
	type NumRow struct {
		Int  int     `tsv:"int"`
		Int8 int8    `tsv:"int8"`
		Real float64 `tsv:"real"`
	}
	data := "int\tint8\treal\n-5\t-10\t-3.14"
	r := NewReader[NumRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, -5, row.Int)
	assert.Equal(t, int8(-10), row.Int8)
	assert.Equal(t, -3.14, row.Real)
}

func TestReader_PointerFields(t *testing.T) {
	type PtrRow struct {
		Name *string `tsv:"name"`
		Age  *int    `tsv:"age"`
	}
	data := "name\tage\nJohn\t25"
	r := NewReader[PtrRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.NotNil(t, row.Name)
	assert.Equal(t, "John", *row.Name)
	assert.NotNil(t, row.Age)
	assert.Equal(t, 25, *row.Age)
}

func TestReader_PointerEmpty(t *testing.T) {
	data := "name\tage\n\t25"
	r := NewReader[PtrRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Nil(t, row.Name)
	assert.NotNil(t, row.Age)
}

func TestReader_HeaderOnly(t *testing.T) {
	data := "name\tage\temail"
	r := NewReader[SimpleRow](strings.NewReader(data))

	_, err := r.Read()
	assert.Equal(t, io.EOF, err)
}

func TestReader_NonExistentColumn(t *testing.T) {
	data := "nonexistent\nvalue"
	r := NewReader[SimpleRow](strings.NewReader(data))

	_, err := r.Read()
	assert.True(t, IsMissingColumnError(err))
	var mc MissingColumnError
	assert.ErrorAs(t, err, &mc)
	assert.Equal(t, "name", mc.Column)
}

func TestReader_OptionalFieldMissing(t *testing.T) {
	data := "name\tage\nJohn\t25"
	r := NewReader[RowWithOptional](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row.Name)
	assert.Equal(t, 25, row.Age)
	assert.Equal(t, "", row.Email)
}

func TestReader_RequiredFieldMissing(t *testing.T) {
	type ReqRow struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}
	data := "name\nJohn"
	r := NewReader[ReqRow](strings.NewReader(data))

	_, err := r.Read()
	assert.True(t, IsMissingColumnError(err))
}

func BenchmarkReader_Next(b *testing.B) {
	data := "name\tage\temail\n" + strings.Repeat("John\t25\tjohn@example.com\n", 1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := NewReader[SimpleRow](strings.NewReader(data))
		for {
			_, err := r.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			assert.NoError(b, err)
		}
	}
}

func BenchmarkReader_Next_SingleRow(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := NewReader[SimpleRow](strings.NewReader("name\tage\temail\nJohn\t25\tjohn@example.com\n"))
		_, err := r.Read()
		assert.NoError(b, err)
	}
}

func TestReader_AllOptionalMissing(t *testing.T) {
	type AllOptRow struct {
		Name string `tsv:"name,optional"`
		Age  int    `tsv:"age,optional"`
	}
	data := "other\tvalue\nvalue1\tvalue2"
	r := NewReader[AllOptRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "", row.Name)
	assert.Equal(t, 0, row.Age)
}

func TestReader_NoColumnsError(t *testing.T) {
	type TestRow struct {
		Name string `tsv:"name"`
	}
	r := NewReader[TestRow](strings.NewReader(""))

	_, err := r.Read()
	assert.Equal(t, io.EOF, err)
}

func TestReader_EmptyHeaderLine(t *testing.T) {
	type TestRow struct {
		Name string `tsv:"name,optional"`
	}
	r := NewReader[TestRow](strings.NewReader("\n"))

	_, err := r.Read()
	assert.Equal(t, ErrNoColumns, err)
}

func TestReader_EmptyColumnName(t *testing.T) {
	type TestRow struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}
	r := NewReader[TestRow](strings.NewReader("name\t\tage"))

	_, err := r.Read()
	assert.True(t, IsEmptyColumnNameError(err))
	var ec EmptyColumnNameError
	assert.ErrorAs(t, err, &ec)
	assert.Equal(t, 1, ec.Index)
}

func TestUnsupportedTypeError(t *testing.T) {
	type UnsupportedRow struct {
		Name string            `tsv:"name"`
		Data map[string]string `tsv:"data"` // map is not supported
	}
	r := NewReader[UnsupportedRow](strings.NewReader("name\tdata\nJohn\tvalue"))
	_, err := r.Read()
	assert.EqualError(t, err, "tsv: unsupported field type: map[string]string")
}

func TestUnsupportedSliceTypeError(t *testing.T) {
	type UnsupportedSliceRow struct {
		Name string `tsv:"name"`
		Data []int  `tsv:"data"` // slices are not supported
	}
	r := NewReader[UnsupportedSliceRow](strings.NewReader("name\tdata\nJohn\t1,2,3"))
	_, err := r.Read()
	assert.EqualError(t, err, "tsv: unsupported field type: []int")
}

func TestValueParseError_Int(t *testing.T) {
	type IntRow struct {
		Age int `tsv:"age"`
	}
	data := "age\nnotanumber"
	r := NewReader[IntRow](strings.NewReader(data))

	_, err := r.Read()
	assert.True(t, IsValueParseError(err))
	var ve ValueParseError
	assert.ErrorAs(t, err, &ve)
	assert.Equal(t, "age", ve.Column)
	assert.Equal(t, "notanumber", ve.Value)
	assert.Equal(t, 2, ve.Row) // row 1 is header, row 2 is data
}

func TestValueParseError_Float(t *testing.T) {
	type FloatRow struct {
		Price float64 `tsv:"price"`
	}
	data := "price\ninvalid"
	r := NewReader[FloatRow](strings.NewReader(data))

	_, err := r.Read()
	assert.True(t, IsValueParseError(err))
	var ve ValueParseError
	assert.ErrorAs(t, err, &ve)
	assert.Equal(t, "price", ve.Column)
	assert.Equal(t, "invalid", ve.Value)
	assert.Equal(t, 2, ve.Row)
}

func TestValueParseError_Uint(t *testing.T) {
	type UintRow struct {
		Count uint `tsv:"count"`
	}
	data := "count\n-5" // negative number for uint
	r := NewReader[UintRow](strings.NewReader(data))

	_, err := r.Read()
	assert.True(t, IsValueParseError(err))
	var ve ValueParseError
	assert.ErrorAs(t, err, &ve)
	assert.Equal(t, "count", ve.Column)
	assert.Equal(t, "-5", ve.Value)
	assert.Equal(t, 2, ve.Row)
}

func TestMissingColumnValueError(t *testing.T) {
	type RequiredRow struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}
	data := "name\tage\nJohn" // missing age column entirely
	r := NewReader[RequiredRow](strings.NewReader(data))

	_, err := r.Read()
	assert.True(t, IsMissingColumnValueError(err))
	var mc MissingColumnValueError
	assert.ErrorAs(t, err, &mc)
	assert.Equal(t, "age", mc.Column)
	assert.Equal(t, 2, mc.Row)
}

func TestMissingColumnValueError_MultipleRows(t *testing.T) {
	type RequiredRow struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}
	// Line 3 "Jane" has only 1 column (no tab at end)
	data := "name\tage\nJohn\t25\nJane\nBob\t30"
	r := NewReader[RequiredRow](strings.NewReader(data))

	_, err := r.Read()
	assert.NoError(t, err) // row 1 (John)

	_, err = r.Read() // row 2 (Jane) - only 1 column, triggers error
	assert.True(t, IsMissingColumnValueError(err))
	var mc MissingColumnValueError
	assert.ErrorAs(t, err, &mc)
	assert.Equal(t, "age", mc.Column)
	assert.Equal(t, 3, mc.Row) // line 1 header, line 2 John, line 3 Jane
}

func TestNoExportedFieldsError(t *testing.T) {
	type NoFieldsRow struct {
		name string // unexported
	}
	r := NewReader[NoFieldsRow](strings.NewReader("name\nJohn"))
	_, err := r.Read()
	assert.ErrorContains(t, err, "no exported fields")
}

func TestEmptyColumnNameError(t *testing.T) {
	type TestRow struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}
	r := NewReader[TestRow](strings.NewReader("name\t\tage"))

	_, err := r.Read()
	assert.True(t, IsEmptyColumnNameError(err))
	var ec EmptyColumnNameError
	assert.ErrorAs(t, err, &ec)
	assert.Equal(t, 1, ec.Index)
}

func TestReader_Iterator(t *testing.T) {
	data := "name\tage\nJohn\t25\nJane\t30"
	r := NewReader[RowWithOptional](strings.NewReader(data))

	var rows []RowWithOptional
	for row, err := range r.Iterator() {
		assert.NoError(t, err)
		rows = append(rows, row)
	}

	assert.Len(t, rows, 2)
	assert.Equal(t, "John", rows[0].Name)
	assert.Equal(t, 25, rows[0].Age)
	assert.Equal(t, "Jane", rows[1].Name)
	assert.Equal(t, 30, rows[1].Age)
}

func TestReader_Iterator_EmptyFile(t *testing.T) {
	r := NewReader[SimpleRow](strings.NewReader(""))

	count := 0
	for range r.Iterator() {
		count++
	}

	assert.Equal(t, 0, count)
}

func TestReader_Iterator_HeaderOnly(t *testing.T) {
	r := NewReader[RowWithOptional](strings.NewReader("name\tage"))

	count := 0
	for range r.Iterator() {
		count++
	}

	assert.Equal(t, 0, count)
}

func TestReader_Iterator_StopsOnError(t *testing.T) {
	type UintRow struct {
		Count uint `tsv:"count"`
	}
	data := "count\n1\ninvalid\n3"
	r := NewReader[UintRow](strings.NewReader(data))

	var rows []UintRow
	var lastErr error
	for row, err := range r.Iterator() {
		if err != nil {
			lastErr = err
			break
		}
		rows = append(rows, row)
	}

	assert.Len(t, rows, 1)
	assert.Equal(t, uint(1), rows[0].Count)
	assert.True(t, IsValueParseError(lastErr))
}

func TestReader_Iterator_YieldBreak(t *testing.T) {
	data := "name\tage\nJohn\t25\nJane\t30\nBob\t40"
	r := NewReader[RowWithOptional](strings.NewReader(data))

	count := 0
	for row := range r.Iterator() {
		count++
		if row.Name == "Jane" {
			break
		}
	}

	assert.Equal(t, 2, count) // John and Jane
}

// CustomDuration implements encoding.TextMarshaler and TextUnmarshaler
type CustomDuration struct {
	Seconds int
}

func (d CustomDuration) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%ds", d.Seconds), nil
}

func (d *CustomDuration) UnmarshalText(text []byte) error {
	s := strings.TrimSuffix(string(text), "s")
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	d.Seconds = n
	return nil
}

type RowWithCustomType struct {
	Name     string         `tsv:"name"`
	Duration CustomDuration `tsv:"duration"`
}

func TestReader_TextUnmarshaler(t *testing.T) {
	data := "name\tduration\nJohn\t30s\nJane\t45s"
	r := NewReader[RowWithCustomType](strings.NewReader(data))

	row1, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row1.Name)
	assert.Equal(t, 30, row1.Duration.Seconds)

	row2, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "Jane", row2.Name)
	assert.Equal(t, 45, row2.Duration.Seconds)
}

func TestWriter_TextMarshaler(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[RowWithCustomType](&buf)

	err := w.Write(RowWithCustomType{Name: "John", Duration: CustomDuration{Seconds: 30}})
	assert.NoError(t, err)
	w.Close()

	assert.Equal(t, "name\tduration\nJohn\t30s\n", buf.String())
}

func TestReader_OptionalFieldDoesNotLeakAcrossRows(t *testing.T) {
	// Row 1 has all three columns; row 2 omits the optional Email column
	// (short row). The leaked-value bug used to surface here as Email
	// retaining "first@example.com" on the second row.
	data := "name\tage\temail\nJohn\t25\tfirst@example.com\nJane\t30"
	r := NewReader[RowWithOptional](strings.NewReader(data))

	row1, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "first@example.com", row1.Email)

	row2, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "Jane", row2.Name)
	assert.Equal(t, 30, row2.Age)
	assert.Equal(t, "", row2.Email, "optional field must not leak from previous row")
}

func TestReader_DuplicateColumn(t *testing.T) {
	data := "name\tname\tage\nJohn\tJ\t25"
	r := NewReader[SimpleRow](strings.NewReader(data))

	_, err := r.Read()
	assert.True(t, IsDuplicateColumnError(err))
	var de DuplicateColumnError
	assert.ErrorAs(t, err, &de)
	assert.Equal(t, "name", de.Column)
	assert.Equal(t, 0, de.FirstIndex)
	assert.Equal(t, 1, de.DuplicateIndex)
}

func TestReader_HeaderErrorIsSticky(t *testing.T) {
	// Header is missing the required "age" column. Subsequent Read calls
	// must keep returning the same error rather than re-parsing the next
	// data line as a header.
	data := "name\nJohn\nJane"
	r := NewReader[SimpleRow](strings.NewReader(data))

	_, err1 := r.Read()
	assert.True(t, IsMissingColumnError(err1))

	_, err2 := r.Read()
	assert.Equal(t, err1, err2)

	_, err3 := r.Read()
	assert.Equal(t, err1, err3)
}

func TestReader_PreservesWhitespaceInValues(t *testing.T) {
	type WSRow struct {
		Name string `tsv:"name"`
	}
	data := "name\n  spaced  "
	r := NewReader[WSRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "  spaced  ", row.Name)
}

func TestReader_LongLineWithOption(t *testing.T) {
	// The default scanner buffer is 64 KiB. WithMaxLineSize raises it so
	// that callers expecting wide rows can opt in.
	type LongRow struct {
		Data string `tsv:"data"`
	}
	long := strings.Repeat("x", 100*1024)
	data := "data\n" + long
	r := NewReader[LongRow](strings.NewReader(data), WithMaxLineSize(1<<20))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, long, row.Data)
}

func TestReader_LongLineExceedsDefault(t *testing.T) {
	// Without WithMaxLineSize, a line over 64 KiB surfaces bufio.ErrTooLong
	// rather than parsing partial data.
	type LongRow struct {
		Data string `tsv:"data"`
	}
	long := strings.Repeat("x", 100*1024)
	data := "data\n" + long
	r := NewReader[LongRow](strings.NewReader(data))

	_, err := r.Read()
	assert.ErrorIs(t, err, bufio.ErrTooLong)
}

func TestReader_BoolInvalidValueErrors(t *testing.T) {
	type BoolRow struct {
		Active bool `tsv:"active"`
	}
	data := "active\nbanana"
	r := NewReader[BoolRow](strings.NewReader(data))

	_, err := r.Read()
	assert.True(t, IsValueParseError(err), "got %v", err)
}

func TestReader_TagSkipDash(t *testing.T) {
	type SkipRow struct {
		Name   string `tsv:"name"`
		Hidden string `tsv:"-"`
		Age    int    `tsv:"age"`
	}
	data := "name\tage\nJohn\t25"
	r := NewReader[SkipRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row.Name)
	assert.Equal(t, 25, row.Age)
	assert.Equal(t, "", row.Hidden)
}

func TestReader_TagOnlyOptionFallsBackToFieldName(t *testing.T) {
	type FallbackRow struct {
		Name string `tsv:",optional"`
	}
	// Field uses its Go name when the tag carries only options.
	data := "Name\nJohn"
	r := NewReader[FallbackRow](strings.NewReader(data))

	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, "John", row.Name)
}

func TestRoundTrip_PlainValues(t *testing.T) {
	rows := []SimpleRow{
		{Name: "John", Age: 25, Email: "john@example.com"},
		{Name: "Jane", Age: 30, Email: "jane@example.com"},
	}

	var buf strings.Builder
	w := NewWriter[SimpleRow](&buf)
	for _, r := range rows {
		assert.NoError(t, w.Write(r))
	}
	assert.NoError(t, w.Close())

	r := NewReader[SimpleRow](strings.NewReader(buf.String()))
	var got []SimpleRow
	for row, err := range r.Iterator() {
		assert.NoError(t, err)
		got = append(got, row)
	}
	assert.Equal(t, rows, got)
}

func TestWriter_RejectsTabInValue(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[SimpleRow](&buf)

	err := w.Write(SimpleRow{Name: "a\tb", Age: 1, Email: "x@y"})
	assert.True(t, IsInvalidCharacterError(err), "got %v", err)
	var ic InvalidCharacterError
	assert.ErrorAs(t, err, &ic)
	assert.Equal(t, "name", ic.Column)
	assert.Equal(t, '\t', ic.Char)

	// No bytes should have been written for the rejected row. The header
	// goes out first (header column names are valid), so after a flush the
	// buffer contains exactly the header line.
	assert.NoError(t, w.Flush())
	assert.Equal(t, "name\tage\temail\n", buf.String())
}

func TestWriter_RejectsNewlineInValue(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[SimpleRow](&buf)

	err := w.Write(SimpleRow{Name: "line1\nline2", Age: 1, Email: "x@y"})
	assert.True(t, IsInvalidCharacterError(err), "got %v", err)
}

func TestWriter_RejectsCarriageReturnInValue(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[SimpleRow](&buf)

	err := w.Write(SimpleRow{Name: "x\ry", Age: 1, Email: "x@y"})
	assert.True(t, IsInvalidCharacterError(err), "got %v", err)
}

func TestWriter_NoCSVQuoting(t *testing.T) {
	// The previous implementation used encoding/csv with Comma='\t', which
	// auto-quoted values containing '"'. The Reader uses naive Split(\t),
	// so quoted output couldn't round-trip. The plain Writer must pass
	// quote characters through untouched.
	type QRow struct {
		Q string `tsv:"q"`
	}
	var buf strings.Builder
	w := NewWriter[QRow](&buf)
	assert.NoError(t, w.Write(QRow{Q: `he said "hi"`}))
	assert.NoError(t, w.Close())

	assert.Equal(t, "q\nhe said \"hi\"\n", buf.String())

	r := NewReader[QRow](strings.NewReader(buf.String()))
	row, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, `he said "hi"`, row.Q)
}

func TestReader_Iterator_StopsOnHeaderError(t *testing.T) {
	// Header errors are sticky. The iterator must yield the error once and
	// then stop — even if the caller doesn't break — to avoid an infinite
	// loop on the same sticky error.
	data := "nonexistent\nvalue"
	r := NewReader[SimpleRow](strings.NewReader(data))

	count := 0
	var lastErr error
	for _, err := range r.Iterator() {
		count++
		lastErr = err
		if count > 5 {
			t.Fatal("iterator did not stop on sticky header error")
		}
	}
	assert.Equal(t, 1, count)
	assert.True(t, IsMissingColumnError(lastErr))
}

func TestReader_Iterator_ContinuesOnRecoverableError(t *testing.T) {
	// ValueParseError describes a single bad row; the scanner has already
	// advanced past it, so iteration must continue with the next row unless
	// the caller chooses to break.
	type IntRow struct {
		Age int `tsv:"age"`
	}
	data := "age\n1\ninvalid\n3"
	r := NewReader[IntRow](strings.NewReader(data))

	var ages []int
	var errs []error
	for row, err := range r.Iterator() {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ages = append(ages, row.Age)
	}
	assert.Equal(t, []int{1, 3}, ages)
	assert.Len(t, errs, 1)
	assert.True(t, IsValueParseError(errs[0]))
}

func TestReader_Iterator_ContinuesOnMissingColumnValue(t *testing.T) {
	// MissingColumnValueError is also recoverable — the short row has been
	// consumed, so the next Read can fetch the row after it.
	type RequiredRow struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}
	data := "name\tage\nJohn\t25\nJane\nBob\t30"
	r := NewReader[RequiredRow](strings.NewReader(data))

	var names []string
	var errs []error
	for row, err := range r.Iterator() {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		names = append(names, row.Name)
	}
	assert.Equal(t, []string{"John", "Bob"}, names)
	assert.Len(t, errs, 1)
	assert.True(t, IsMissingColumnValueError(errs[0]))
}

func TestReader_Iterator_StopsOnScannerError(t *testing.T) {
	// bufio.ErrTooLong on a data line is sticky — the scanner won't produce
	// further tokens. Iterator must yield once and stop, even if the caller
	// doesn't break.
	type LongRow struct {
		Data string `tsv:"data"`
	}
	long := strings.Repeat("x", 100*1024)
	data := "data\nshort\n" + long + "\nshort2"
	r := NewReader[LongRow](strings.NewReader(data)) // default 64 KiB max

	var rows []LongRow
	var errs []error
	count := 0
	for row, err := range r.Iterator() {
		count++
		if count > 10 {
			t.Fatal("iterator did not stop on sticky scanner error")
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		rows = append(rows, row)
	}
	assert.Len(t, rows, 1)
	assert.Equal(t, "short", rows[0].Data)
	assert.Len(t, errs, 1)
	assert.ErrorIs(t, errs[0], bufio.ErrTooLong)
}
