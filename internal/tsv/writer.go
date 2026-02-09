package tsv

import (
	"encoding/csv"
	"io"
	"reflect"
)

// Writer writes TSV data to an io.Writer from struct values.
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
//	w := tsv.NewWriter[Person](file)
//	w.Write(Person{Name: "John", Age: 30})
//	w.Close()
type Writer[T any] struct {
	writer      *csv.Writer
	typeInfo    *typeInfo
	wroteHeader bool
	values      []string // Reused buffer for row values
	row         int      // Number of data rows written (excludes header)
}

// NewWriter creates a new Writer for type T that writes to w.
func NewWriter[T any](w io.Writer) *Writer[T] {
	csvWriter := csv.NewWriter(w)
	csvWriter.Comma = '\t'

	return &Writer[T]{
		writer: csvWriter,
	}
}

func (w *Writer[T]) init() error {
	var err error
	w.typeInfo, err = getTypeInfo[T]()
	if err == nil {
		w.values = make([]string, len(w.typeInfo.fields))
	}
	return err
}

// writeHeader writes the header row with column names.
func (w *Writer[T]) writeHeader() error {
	for i, fi := range w.typeInfo.fields {
		w.values[i] = fi.name
	}
	w.wroteHeader = true
	return w.writer.Write(w.values)
}

// Write writes a single row to the TSV output.
// The first call writes the header row automatically.
// Subsequent calls write data rows.
func (w *Writer[T]) Write(row T) error {
	if !w.wroteHeader {
		if err := w.init(); err != nil {
			return err
		}
		if err := w.writeHeader(); err != nil {
			return err
		}
	}

	w.row++

	rv := reflect.ValueOf(row)
	for i, fi := range w.typeInfo.fields {
		s, err := formatValue(rv.FieldByIndex(fi.index))
		if err != nil {
			return err
		}
		w.values[i] = s
	}

	return w.writer.Write(w.values)
}

// Flush flushes any buffered data to the underlying writer.
func (w *Writer[T]) Flush() error {
	w.writer.Flush()
	return w.writer.Error()
}

// Close flushes the writer and returns any error.
// Always call Close when finished writing.
func (w *Writer[T]) Close() error {
	return w.Flush()
}
