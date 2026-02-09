package tsv

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriter_Basic(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[SimpleRow](&buf)

	row := SimpleRow{Name: "John", Age: 25, Email: "john@example.com"}
	err := w.Write(row)
	assert.NoError(t, err)
	w.Close()

	result := buf.String()
	expected := "name\tage\temail\nJohn\t25\tjohn@example.com\n"
	assert.Equal(t, expected, result)
}

func TestWriter_MultipleRows(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[SimpleRow](&buf)

	err := w.Write(SimpleRow{Name: "John", Age: 25, Email: "john@example.com"})
	assert.NoError(t, err)

	err = w.Write(SimpleRow{Name: "Jane", Age: 30, Email: "jane@example.com"})
	assert.NoError(t, err)

	w.Close()

	result := buf.String()
	expected := "name\tage\temail\nJohn\t25\tjohn@example.com\nJane\t30\tjane@example.com\n"
	assert.Equal(t, expected, result)
}

func TestWriter_EmbeddedStruct(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[RowWithEmbedded](&buf)

	row := RowWithEmbedded{
		EmbeddedBase: EmbeddedBase{Created: "2024-01-01"},
		Name:         "John",
	}
	err := w.Write(row)
	assert.NoError(t, err)
	w.Close()

	result := buf.String()
	expected := "created\tname\n2024-01-01\tJohn\n"
	assert.Equal(t, expected, result)
}

func TestWriter_AllTypes(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[RowWithAllTypes](&buf)

	row := RowWithAllTypes{String: "hello", Int: 42, Bool: true}
	err := w.Write(row)
	assert.NoError(t, err)
	w.Close()

	result := buf.String()
	assert.Contains(t, result, "hello")
	assert.Contains(t, result, "42")
	assert.Contains(t, result, "true")
}

func TestWriter_PointerFields(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[PtrRow](&buf)

	name := "John"
	age := 25
	row := PtrRow{Name: &name, Age: &age}
	err := w.Write(row)
	assert.NoError(t, err)
	w.Close()

	result := buf.String()
	assert.Contains(t, result, "John")
	assert.Contains(t, result, "25")
}

func TestWriter_NilPointer(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[PtrRow](&buf)

	row := PtrRow{Name: nil, Age: nil}
	err := w.Write(row)
	assert.NoError(t, err)
	w.Close()

	result := buf.String()
	assert.Equal(t, "name\tage\n\t\n", result)
}

func TestWriter_Flush(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[SimpleRow](&buf)

	w.Flush()

	row := SimpleRow{Name: "John", Age: 25, Email: "john@example.com"}
	err := w.Write(row)
	assert.NoError(t, err)

	w.Flush()
	w.Close()
}

func TestWriter_EmptyClose(t *testing.T) {
	var buf strings.Builder
	w := NewWriter[SimpleRow](&buf)
	_ = w.Write(SimpleRow{Name: "", Age: 0, Email: ""})
	w.Close()

	result := buf.String()
	assert.Equal(t, "name\tage\temail\n\t0\t\n", result)
}

func TestWriter_FieldCache(t *testing.T) {
	type CacheRow1 struct {
		Name string `tsv:"col1"`
		Age  int    `tsv:"col2"`
	}

	var buf1 strings.Builder
	w1 := NewWriter[CacheRow1](&buf1)

	err1 := w1.Write(CacheRow1{Name: "John", Age: 25})
	assert.NoError(t, err1)

	w1.Close()

	// Verify output structure
	result := buf1.String()
	assert.Equal(t, "col1\tcol2\nJohn\t25\n", result)
}
