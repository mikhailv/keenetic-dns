package tsv

import (
	"encoding"
	"errors"
	"reflect"
	"strconv"
	"strings"
)

var (
	textMarshallerType   = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	textUnmarshallerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// parseValue converts a string value to the specified reflect.Type.
// It handles built-in types and types implementing encoding.TextUnmarshaler.
// Returns a reflect.Value that can be set on a struct field.
func parseValue(val string, typ reflect.Type) (reflect.Value, error) {
	// Empty string maps to zero value for pointer types
	if val == "" && typ.Kind() == reflect.Ptr {
		return reflect.Zero(typ), nil
	}

	// Check for TextUnmarshaler interface
	if tm, ok := reflect.New(typ).Interface().(encoding.TextUnmarshaler); ok {
		if err := tm.UnmarshalText([]byte(val)); err != nil {
			return reflect.Value{}, err
		}
		result := reflect.ValueOf(tm)
		// If result is pointer and target type is not pointer, dereference
		if result.Kind() == reflect.Ptr && typ.Kind() != reflect.Ptr {
			return result.Elem(), nil
		}
		return result, nil
	}

	switch typ.Kind() { //nolint:exhaustive // `default` will handle missing cases
	case reflect.String:
		return reflect.ValueOf(val), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(i).Convert(typ), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(u).Convert(typ), nil
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(f).Convert(typ), nil
	case reflect.Bool:
		b := strings.ToLower(val) == "true" || val == "1"
		return reflect.ValueOf(b), nil
	case reflect.Ptr:
		elem, err := parseValue(val, typ.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(typ.Elem())
		ptr.Elem().Set(elem)
		return ptr, nil
	default:
		return reflect.Value{}, errors.New("tsv: unsupported field type: " + typ.String())
	}
}

// formatValue converts a struct field value to a string.
// It handles built-in types and types implementing encoding.TextMarshaler.
// Returns the string representation and any error from MarshalText.
func formatValue(v reflect.Value) (string, error) {
	// Check for TextMarshaler interface first
	if v.CanInterface() {
		if tm, ok := v.Interface().(encoding.TextMarshaler); ok {
			bs, err := tm.MarshalText()
			return string(bs), err
		}
	}

	typ := v.Type()
	switch typ.Kind() { //nolint:exhaustive // `default` will handle missing cases
	case reflect.String:
		return v.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Ptr:
		if v.IsNil() {
			return "", nil
		}
		return formatValue(v.Elem())
	default:
		return "", errors.New("tsv: unsupported field type: " + typ.String())
	}
}

// checkType validates that a reflect.Type is supported by the TSV package.
// Returns an error if the type is not supported.
// Supported types: string, all int/uint variants, float variants, bool,
// pointers to supported types, and types implementing TextMarshaler/TextUnmarshaler.
func checkType(typ reflect.Type) error {
	// Check if type implements TextMarshaler/TextUnmarshaler
	if typ.Implements(textMarshallerType) || typ.Implements(textUnmarshallerType) {
		return nil
	}
	// Also check pointer receiver implementations
	if typ.Kind() != reflect.Ptr {
		ptyp := reflect.PointerTo(typ)
		if ptyp.Implements(textMarshallerType) || ptyp.Implements(textUnmarshallerType) {
			return nil
		}
	}

	switch typ.Kind() { //nolint:exhaustive // `default` will handle missing cases
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.Bool:
		return nil
	case reflect.Ptr:
		return checkType(typ.Elem())
	default:
		return errors.New("tsv: unsupported field type: " + typ.String())
	}
}

// zeroVal returns the zero value of type T.
func zeroVal[T any]() T {
	var zero T
	return zero
}
