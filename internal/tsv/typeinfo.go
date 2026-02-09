package tsv

import (
	"errors"
	"reflect"
	"strings"
	"sync"
)

// typeCache stores type metadata for all struct types used with Reader and Writer.
// This avoids repeatedly computing field information for the same types.
var typeCache sync.Map // map[reflect.Type]*typeInfo

// fieldInfo contains metadata for a single struct field.
type fieldInfo struct {
	name     string              // Column name (from tag or field name)
	field    reflect.StructField // The actual struct field
	optional bool                // True if field is marked as optional
	index    []int               // Full index path for FieldByIndex (handles embedded structs)
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

		// Validate field type is supported
		if err := checkType(f.Type); err != nil {
			return err
		}

		fi := fieldInfo{
			name:  f.Name,
			field: f,
			index: newIndex,
		}

		// Parse ts tag: "column_name" or "column_name,optional"
		if tag := f.Tag.Get("tsv"); tag != "" {
			parts := strings.Split(tag, ",")
			fi.name = strings.TrimSpace(parts[0])
			for _, p := range parts[1:] {
				if strings.TrimSpace(p) == "optional" {
					fi.optional = true
					break
				}
			}
		}

		info.fields = append(info.fields, fi)
	}
	return nil
}
