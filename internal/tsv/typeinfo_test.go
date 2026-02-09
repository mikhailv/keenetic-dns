package tsv

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTypeInfo_Struct(t *testing.T) {
	type TestRow struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}

	info, err := getTypeInfo[TestRow]()
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, reflect.TypeOf(TestRow{}), info.typ)
	assert.Len(t, info.fields, 2)
}

func TestGetTypeInfo_Embedded(t *testing.T) {
	type Embedded struct {
		Created string `tsv:"created"`
	}

	type WithEmbedded struct {
		Embedded
		Name string `tsv:"name"`
	}

	info, err := getTypeInfo[WithEmbedded]()
	assert.NoError(t, err)
	assert.Len(t, info.fields, 2) // Both embedded and direct fields
}

func TestGetTypeInfo_Optional(t *testing.T) {
	type OptionalRow struct {
		Name    string `tsv:"name"`
		Missing string `tsv:"missing,optional"`
		Age     int    `tsv:"age"`
	}

	info, err := getTypeInfo[OptionalRow]()
	assert.NoError(t, err)

	// Find the "missing" field (lowercase from tag) and check optional flag
	var missingField fieldInfo
	for _, f := range info.fields {
		if f.name == "missing" {
			missingField = f
			break
		}
	}
	assert.True(t, missingField.optional)
}

func TestGetTypeInfo_NoTagUsesFieldName(t *testing.T) {
	type NoTagRow struct {
		Name string `tsv:"name"`
		Age  int
	}

	info, err := getTypeInfo[NoTagRow]()
	assert.NoError(t, err)

	// Find the "Age" field (without tag)
	var ageField fieldInfo
	for _, f := range info.fields {
		if f.field.Name == "Age" {
			ageField = f
			break
		}
	}
	assert.Equal(t, "Age", ageField.name)
}

func TestGetTypeInfo_Cache(t *testing.T) {
	type Row1 struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}

	type Row2 struct {
		Name string `tsv:"name"`
		Age  int    `tsv:"age"`
	}

	info1, _ := getTypeInfo[Row1]()
	info2, _ := getTypeInfo[Row1]()
	info3, _ := getTypeInfo[Row2]()

	assert.Same(t, info1, info2, "same type should return same cached instance")
	assert.NotSame(t, info1, info3, "different types should return different instances")
}

func TestBuildTypeInfo_NotStruct(t *testing.T) {
	_, err := buildTypeInfo(reflect.TypeOf("string"))
	assert.ErrorContains(t, err, "expected struct type")
}

func TestBuildTypeInfo_NoExportedFields(t *testing.T) {
	type NoExported struct {
		name string // unexported
	}
	_, err := buildTypeInfo(reflect.TypeOf(NoExported{}))
	assert.ErrorContains(t, err, "no exported fields")
}

func TestCollectFields_AllTypes(t *testing.T) {
	type AllTypesRow struct {
		String  string  `tsv:"str"`
		Int     int     `tsv:"int"`
		Int8    int8    `tsv:"int8"`
		Int16   int16   `tsv:"int16"`
		Int32   int32   `tsv:"int32"`
		Int64   int64   `tsv:"int64"`
		Uint    uint    `tsv:"uint"`
		Uint8   uint8   `tsv:"uint8"`
		Uint16  uint16  `tsv:"uint16"`
		Uint32  uint32  `tsv:"uint32"`
		Uint64  uint64  `tsv:"uint64"`
		Float32 float32 `tsv:"float32"`
		Float64 float64 `tsv:"float64"`
		Bool    bool    `tsv:"bool"`
	}

	info, err := buildTypeInfo(reflect.TypeOf(AllTypesRow{}))
	assert.NoError(t, err)
	assert.Len(t, info.fields, 14) // All 14 fields
}

func TestCheckType_Supported(t *testing.T) {
	tests := []struct {
		name string
		typ  interface{}
	}{
		{"string", ""},
		{"int", 0},
		{"int8", int8(0)},
		{"int16", int16(0)},
		{"int32", int32(0)},
		{"int64", int64(0)},
		{"uint", uint(0)},
		{"uint8", uint8(0)},
		{"uint16", uint16(0)},
		{"uint32", uint32(0)},
		{"uint64", uint64(0)},
		{"float32", float32(0)},
		{"float64", float64(0)},
		{"bool", false},
		{"string ptr", new(string)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkType(reflect.TypeOf(tt.typ))
			assert.NoError(t, err)
		})
	}
}

func TestCheckType_Unsupported(t *testing.T) {
	type CustomMap map[string]string
	type CustomSlice []int
	type CustomStruct struct{ X int }

	tests := []struct {
		name string
		typ  reflect.Type
	}{
		{"map", reflect.TypeOf(CustomMap(nil))},
		{"slice", reflect.TypeOf(CustomSlice(nil))},
		{"struct", reflect.TypeOf(CustomStruct{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkType(tt.typ)
			assert.ErrorContains(t, err, "unsupported field type")
		})
	}
}

// TextMarshalerType for testing
type TextValue struct {
	Value string
}

func (t TextValue) MarshalText() ([]byte, error) {
	return []byte(t.Value), nil
}

func (t *TextValue) UnmarshalText(text []byte) error {
	t.Value = string(text)
	return nil
}

func TestCheckType_TextMarshaler(t *testing.T) {
	// Value type with TextMarshaler/Unmarshaler
	err := checkType(reflect.TypeOf(TextValue{}))
	assert.NoError(t, err)

	// Pointer to type with TextMarshaler/Unmarshaler
	err = checkType(reflect.TypeOf(&TextValue{}))
	assert.NoError(t, err)
}

func TestFieldInfo_IndexPath(t *testing.T) {
	type Inner struct {
		X int `tsv:"x"`
	}

	type Outer struct {
		Inner
		Y int `tsv:"y"`
	}

	info, err := getTypeInfo[Outer]()
	assert.NoError(t, err)

	// Check that embedded field has correct index path
	var innerField, outerField fieldInfo
	for _, f := range info.fields {
		if f.field.Name == "X" {
			innerField = f
		}
		if f.field.Name == "Y" {
			outerField = f
		}
	}

	assert.Len(t, innerField.index, 2) // [0, 0] for embedded field
	assert.Len(t, outerField.index, 1) // [1] for direct field
}
