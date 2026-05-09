package tsv

import (
	"bufio"
	"io"
	"reflect"
	"strings"
)

// Writer writes TSV data to an io.Writer from struct values.
// The generic type parameter T must be a struct type with exported fields
// tagged with `tsv` tags for column names.
//
// Values containing tab, newline, or carriage return characters are rejected
// with InvalidCharacterError — TSV has no escape syntax, so writing such a
// value would corrupt the output. Validation happens before any bytes are
// written for the row, so a rejected row leaves the output stream untouched.
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
//
// A Writer is not safe for concurrent use.
type Writer[T any] struct {
	writer      *bufio.Writer
	typeInfo    *typeInfo
	wroteHeader bool
	values      []string // Reused per-row scratch buffer
	row         int      // Number of data rows successfully written (excludes header)
}

// NewWriter creates a new Writer for type T that writes to w.
func NewWriter[T any](w io.Writer) *Writer[T] {
	return &Writer[T]{
		writer: bufio.NewWriter(w),
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

// Write writes a single row to the TSV output.
// The first call writes the header row automatically.
// Subsequent calls write data rows.
func (w *Writer[T]) Write(row T) error {
	if !w.wroteHeader {
		if err := w.init(); err != nil {
			return err
		}
		for i, fi := range w.typeInfo.fields {
			if err := validateCell(fi.name, fi.name, 0); err != nil {
				return err
			}
			w.values[i] = fi.name
		}
		if err := w.writeRow(w.values); err != nil {
			return err
		}
		w.wroteHeader = true
	}

	nextRow := w.row + 1
	rv := reflect.ValueOf(row)
	for i, fi := range w.typeInfo.fields {
		s, err := fi.format(rv.FieldByIndex(fi.index))
		if err != nil {
			return err
		}
		if err := validateCell(fi.name, s, nextRow); err != nil {
			return err
		}
		w.values[i] = s
	}
	if err := w.writeRow(w.values); err != nil {
		return err
	}
	w.row = nextRow
	return nil
}

// writeRow writes the given cells joined by tabs and terminated by a newline.
func (w *Writer[T]) writeRow(cells []string) error {
	for i, c := range cells {
		if i > 0 {
			if err := w.writer.WriteByte('\t'); err != nil {
				return err
			}
		}
		if _, err := w.writer.WriteString(c); err != nil {
			return err
		}
	}
	return w.writer.WriteByte('\n')
}

// Flush flushes any buffered data to the underlying writer.
func (w *Writer[T]) Flush() error {
	return w.writer.Flush()
}

// Close flushes any buffered data. It does not close the underlying io.Writer
// — the caller retains ownership of it.
func (w *Writer[T]) Close() error {
	return w.Flush()
}

// validateCell ensures a value contains no characters that would corrupt the
// TSV format.
func validateCell(column, val string, row int) error {
	if i := strings.IndexAny(val, "\t\n\r"); i >= 0 {
		return InvalidCharacterError{Column: column, Char: rune(val[i]), Row: row}
	}
	return nil
}
