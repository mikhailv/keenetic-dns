package tsv

import (
	"encoding"
	"errors"
	"reflect"
	"strconv"
)

var (
	textMarshallerType   = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	textUnmarshallerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// buildParser returns a function that parses a string into a reflect.Value of
// the given type. The returned value is suitable for reflect.Value.Set on a
// struct field. Returns an error if the type is not supported.
func buildParser(typ reflect.Type) (func(string) (reflect.Value, error), error) {
	if typ.Kind() == reflect.Ptr {
		elemTyp := typ.Elem()
		// If typ itself (i.e. *elemTyp) implements TextUnmarshaler, allocate
		// the result pointer directly and unmarshal into it — saves an extra
		// allocation+copy compared to recursing through the elem parser.
		if typ.Implements(textUnmarshallerType) {
			return func(val string) (reflect.Value, error) {
				if val == "" {
					return reflect.Zero(typ), nil
				}
				v := reflect.New(elemTyp)
				if err := v.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(val)); err != nil { //nolint:errcheck // checked above
					return reflect.Value{}, err
				}
				return v, nil
			}, nil
		}
		elemParse, err := buildParser(elemTyp)
		if err != nil {
			return nil, err
		}
		return func(val string) (reflect.Value, error) {
			if val == "" {
				return reflect.Zero(typ), nil
			}
			elem, err := elemParse(val)
			if err != nil {
				return reflect.Value{}, err
			}
			ptr := reflect.New(elemTyp)
			ptr.Elem().Set(elem)
			return ptr, nil
		}, nil
	}

	// TextUnmarshaler is conventionally implemented with a pointer receiver,
	// so check *typ rather than typ itself.
	if reflect.PointerTo(typ).Implements(textUnmarshallerType) {
		return func(val string) (reflect.Value, error) {
			v := reflect.New(typ)
			if err := v.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(val)); err != nil { //nolint:errcheck // checked above
				return reflect.Value{}, err
			}
			return v.Elem(), nil
		}, nil
	}

	switch typ.Kind() { //nolint:exhaustive // default handles missing cases
	case reflect.String:
		return func(val string) (reflect.Value, error) {
			return reflect.ValueOf(val), nil
		}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(val string) (reflect.Value, error) {
			i, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(i).Convert(typ), nil
		}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(val string) (reflect.Value, error) {
			u, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(u).Convert(typ), nil
		}, nil
	case reflect.Float32, reflect.Float64:
		return func(val string) (reflect.Value, error) {
			f, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(f).Convert(typ), nil
		}, nil
	case reflect.Bool:
		return func(val string) (reflect.Value, error) {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(b), nil
		}, nil
	default:
		return nil, errors.New("tsv: unsupported field type: " + typ.String())
	}
}

// buildFormatter returns a function that formats a reflect.Value of the given
// type as a string. Returns an error if the type is not supported.
func buildFormatter(typ reflect.Type) (func(reflect.Value) (string, error), error) {
	// Direct TextMarshaler (value receiver, or pointer type that satisfies it).
	if typ.Implements(textMarshallerType) {
		return func(v reflect.Value) (string, error) {
			bs, err := v.Interface().(encoding.TextMarshaler).MarshalText() //nolint:errcheck // checked above
			return string(bs), err
		}, nil
	}
	// Pointer-receiver TextMarshaler on a value type: copy to addressable
	// storage so we can call the method.
	if typ.Kind() != reflect.Ptr && reflect.PointerTo(typ).Implements(textMarshallerType) {
		return func(v reflect.Value) (string, error) {
			tmp := reflect.New(typ)
			tmp.Elem().Set(v)
			bs, err := tmp.Interface().(encoding.TextMarshaler).MarshalText() //nolint:errcheck // checked above
			return string(bs), err
		}, nil
	}

	if typ.Kind() == reflect.Ptr {
		elemFormat, err := buildFormatter(typ.Elem())
		if err != nil {
			return nil, err
		}
		return func(v reflect.Value) (string, error) {
			if v.IsNil() {
				return "", nil
			}
			return elemFormat(v.Elem())
		}, nil
	}

	switch typ.Kind() { //nolint:exhaustive // default handles missing cases
	case reflect.String:
		return func(v reflect.Value) (string, error) { return v.String(), nil }, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(v reflect.Value) (string, error) {
			return strconv.FormatInt(v.Int(), 10), nil
		}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(v reflect.Value) (string, error) {
			return strconv.FormatUint(v.Uint(), 10), nil
		}, nil
	case reflect.Float32, reflect.Float64:
		return func(v reflect.Value) (string, error) {
			return strconv.FormatFloat(v.Float(), 'f', -1, 64), nil
		}, nil
	case reflect.Bool:
		return func(v reflect.Value) (string, error) {
			return strconv.FormatBool(v.Bool()), nil
		}, nil
	default:
		return nil, errors.New("tsv: unsupported field type: " + typ.String())
	}
}

// checkType validates that a reflect.Type is supported by the TSV package
// (for both reading and writing). Used by tests; production code uses the
// builders directly via buildTypeInfo.
func checkType(typ reflect.Type) error {
	if _, err := buildParser(typ); err != nil {
		return err
	}
	if _, err := buildFormatter(typ); err != nil {
		return err
	}
	return nil
}

// zeroVal returns the zero value of type T.
func zeroVal[T any]() T {
	var zero T
	return zero
}
