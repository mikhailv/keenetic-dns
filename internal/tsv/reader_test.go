package tsv

import (
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

func TestReader_ScanError(t *testing.T) {
	r := NewReader[SimpleRow](strings.NewReader("name\tage\n"))
	_, _ = r.Read()
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
	return []byte(fmt.Sprintf("%ds", d.Seconds)), nil
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
