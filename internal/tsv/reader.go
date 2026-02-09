package tsv

import (
	"bufio"
	"errors"
	"io"
	"iter"
	"reflect"
	"strings"
)

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
type Reader[T any] struct {
	scanner  *bufio.Scanner
	typeInfo *typeInfo
	colMap   map[int]int // field index -> column index in header; nil until header is parsed
	value    T           // Reused buffer for row values
	row      int         // Current row number (1-based, header is line 1)
}

// NewReader creates a new Reader for type T that reads from r.
func NewReader[T any](r io.Reader) *Reader[T] {
	return &Reader[T]{
		scanner: bufio.NewScanner(r),
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
func (r *Reader[T]) Read() (T, error) {
	if r.colMap == nil {
		r.row++
		if err := r.init(); err != nil {
			return zeroVal[T](), err
		}
		if line, err := r.scanNextLine(); err != nil {
			return zeroVal[T](), err
		} else if err := r.parseHeader(line); err != nil {
			return zeroVal[T](), err
		}
	}

	line, err := r.scanNextLine()
	if err != nil {
		return zeroVal[T](), err
	}

	r.row++

	rowVals := strings.Split(line, "\t")

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

		val := strings.TrimSpace(rowVals[colIdx])

		fieldVal, err := parseValue(val, f.field.Type)
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
	headerLine = strings.TrimSpace(headerLine)
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
// Example usage:
//
//	for row, err := range r.Iterator() {
//		if err != nil {
//			// handle the error, it can be ValueParseError, MissingColumnValueError or i/o error.
//		} else {
//			// use row
//		}
//	}
//
// The iterator yields (row, error) pairs. Iteration stops when:
//   - io.EOF is returned (normal completion)
//   - The yield function returns false
//
// Note: Errors other than io.EOF are yielded to the caller.
// The caller should check for errors and break the loop if needed.
func (r *Reader[T]) Iterator() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for {
			row, err := r.Read()
			if err != nil && errors.Is(err, io.EOF) {
				return
			}
			if !yield(row, err) {
				return
			}
		}
	}
}
