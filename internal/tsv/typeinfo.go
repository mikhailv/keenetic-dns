package tsv

import (
	"errors"
	"reflect"
	"strings"
	"sync"
)

// typeCache stores type metadata for all struct types used with Reader and
// Writer. The cache lives for the process lifetime; in normal use callers
// register a small fixed set of struct types so unbounded growth is not a
// concern.
var typeCache sync.Map // map[reflect.Type]*typeInfo

// fieldInfo contains metadata for a single struct field.
type fieldInfo struct {
	name     string                              // Column name (from tag or field name)
	field    reflect.StructField                 // The actual struct field
	optional bool                                // True if field is marked as optional
	index    []int                               // Full index path for FieldByIndex (handles embedded structs)
	parse    func(string) (reflect.Value, error) // Parses a raw cell value into the field
	format   func(reflect.Value) (string, error) // Formats a field value into a cell
}

// typeInfo contains metadata for a struct type.
type typeInfo struct {
	typ    reflect.Type // The reflect.Type of the struct
	fields []fieldInfo  // Information about each exported field
}

// getTypeInfo returns the cached typeInfo for type T, building it if necessary.
func getTypeInfo[T any]() (*typeInfo, error) {
	typ := reflect.TypeOf(zeroVal[T]())
	if info, ok := typeCache.Load(typ); ok {
		return info.(*typeInfo), nil //nolint:errcheck // expected type
	}
	info, err := buildTypeInfo(typ)
	if err == nil {
		typeCache.Store(typ, info)
	}
	return info, err
}

// buildTypeInfo creates type metadata for a struct type.
func buildTypeInfo(typ reflect.Type) (*typeInfo, error) {
	if typ.Kind() != reflect.Struct {
		return nil, errors.New("tsv: expected struct type")
	}
	info := &typeInfo{
		typ:    typ,
		fields: make([]fieldInfo, 0, typ.NumField()),
	}
	if err := collectFields(typ, nil, info); err != nil {
		return nil, err
	}
	if len(info.fields) == 0 {
		return nil, errors.New("tsv: no exported fields in type: " + typ.String())
	}
	return info, nil
}

// collectFields recursively collects field information from a struct type.
// It handles embedded (anonymous) fields by including them in the flattened field list.
func collectFields(st reflect.Type, index []int, info *typeInfo) error {
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		newIndex := append(append([]int{}, index...), i)

		// Recursively handle embedded structs
		if f.Anonymous {
			if err := collectFields(f.Type, newIndex, info); err != nil {
				return err
			}
			continue
		}

		// Skip unexported fields
		if !f.IsExported() {
			continue
		}

		// Parse tsv tag: "column_name", "column_name,optional", or "-" to skip.
		// An empty name (e.g. ",optional") falls back to the field name.
		name := f.Name
		optional := false
		if tag, ok := f.Tag.Lookup("tsv"); ok {
			parts := strings.Split(tag, ",")
			tagName := strings.TrimSpace(parts[0])
			if tagName == "-" {
				continue
			}
			if tagName != "" {
				name = tagName
			}
			for _, p := range parts[1:] {
				if strings.TrimSpace(p) == "optional" {
					optional = true
					break
				}
			}
		}

		parser, err := buildParser(f.Type)
		if err != nil {
			return err
		}
		formatter, err := buildFormatter(f.Type)
		if err != nil {
			return err
		}

		info.fields = append(info.fields, fieldInfo{
			name:     name,
			field:    f,
			optional: optional,
			index:    newIndex,
			parse:    parser,
			format:   formatter,
		})
	}
	return nil
}
