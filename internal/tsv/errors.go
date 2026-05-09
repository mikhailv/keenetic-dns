// Package tsv provides generic TSV (Tab-Separated Values) reader and writer.
//
// The package supports struct tags to map between TSV columns and struct fields.
// By default, the column name is taken from the struct field name, but can be
// overridden using the `tsv` tag. Fields can be marked as optional to allow
// missing columns.
//
// Supported field types: string, all integer types, all float types, bool,
// pointers to supported types, and types implementing encoding.TextMarshaler/TextUnmarshaler.
//
// Example:
//
//	type Person struct {
//	    Name string `tsv:"name"`
//	    Age  int    `tsv:"age"`
//	}
//
//	// Reading
//	r := tsv.NewReader[Person](file)
//	for row, err := range r.Iterator() {
//	    if err != nil {
//	        // handle error
//	    }
//	    // use row
//	}
//
//	// Writing
//	w := tsv.NewWriter[Person](output)
//	w.Write(Person{Name: "John", Age: 30})
package tsv

import (
	"errors"
	"fmt"
)

// ErrNoColumns is returned when the TSV header line is empty.
var ErrNoColumns = errors.New("tsv: no columns found")

// MissingColumnError indicates that a required column is missing from the header.
type MissingColumnError struct {
	Column string // Name of the missing column
}

// Error returns a human-readable error message.
func (e MissingColumnError) Error() string {
	return "tsv: missing required column: " + e.Column
}

// IsMissingColumnError reports whether the error is a MissingColumnError.
func IsMissingColumnError(err error) bool {
	var mc MissingColumnError
	return errors.As(err, &mc)
}

// MissingColumnValueError indicates that a required column value is missing from a data row.
type MissingColumnValueError struct {
	Column string // Name of the column with missing value
	Row    int    // Line number in the file (1-based, header is line 1)
}

// Error returns a human-readable error message.
func (e MissingColumnValueError) Error() string {
	return fmt.Sprintf("tsv: missing required %q column value at %d row", e.Column, e.Row)
}

// IsMissingColumnValueError reports whether the error is a MissingColumnValueError.
func IsMissingColumnValueError(err error) bool {
	var mc MissingColumnValueError
	return errors.As(err, &mc)
}

// ValueParseError indicates that a column value could not be parsed.
type ValueParseError struct {
	Column string // Name of the column with invalid value
	Value  string // The invalid value that could not be parsed
	Row    int    // Line number in the file (1-based, header is line 1)
	Err    error  // Underlying parse error
}

// Error returns a human-readable error message.
func (e ValueParseError) Error() string {
	return fmt.Sprintf("tsv: failed to parse %q column value %q at %d row: %v", e.Column, e.Value, e.Row, e.Err)
}

// IsValueParseError reports whether the error is a ValueParseError.
func IsValueParseError(err error) bool {
	var mc ValueParseError
	return errors.As(err, &mc)
}

// EmptyColumnNameError indicates that a column name in the header is empty.
type EmptyColumnNameError struct {
	Index int // Index of the empty column name (0-based)
}

// Error returns a human-readable error message.
func (e EmptyColumnNameError) Error() string {
	return "tsv: empty column name at index: " + fmt.Sprint(e.Index)
}

// IsEmptyColumnNameError reports whether the error is an EmptyColumnNameError.
func IsEmptyColumnNameError(err error) bool {
	var ec EmptyColumnNameError
	return errors.As(err, &ec)
}

// DuplicateColumnError indicates that the same column name appears twice in the header.
type DuplicateColumnError struct {
	Column         string // Name of the duplicated column
	FirstIndex     int    // Index where the column first appeared (0-based)
	DuplicateIndex int    // Index where the duplicate appeared (0-based)
}

// Error returns a human-readable error message.
func (e DuplicateColumnError) Error() string {
	return fmt.Sprintf("tsv: duplicate column %q at indices %d and %d", e.Column, e.FirstIndex, e.DuplicateIndex)
}

// IsDuplicateColumnError reports whether the error is a DuplicateColumnError.
func IsDuplicateColumnError(err error) bool {
	var de DuplicateColumnError
	return errors.As(err, &de)
}

// InvalidCharacterError indicates that a value to be written contains a character
// that would corrupt the TSV format (tab, newline, or carriage return).
type InvalidCharacterError struct {
	Column string // Name of the column being written
	Char   rune   // The offending character
	Row    int    // Data row number (0 for header)
}

// Error returns a human-readable error message.
func (e InvalidCharacterError) Error() string {
	if e.Row == 0 {
		return fmt.Sprintf("tsv: invalid character %q in header column %q", e.Char, e.Column)
	}
	return fmt.Sprintf("tsv: invalid character %q in column %q at row %d", e.Char, e.Column, e.Row)
}

// IsInvalidCharacterError reports whether the error is an InvalidCharacterError.
func IsInvalidCharacterError(err error) bool {
	var ic InvalidCharacterError
	return errors.As(err, &ic)
}
