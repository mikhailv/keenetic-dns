package tsv

type SimpleRow struct {
	Name  string `tsv:"name"`
	Age   int    `tsv:"age"`
	Email string `tsv:"email"`
}

type RowWithOptional struct {
	Name  string `tsv:"name"`
	Age   int    `tsv:"age"`
	Email string `tsv:"email,optional"`
}

type EmbeddedBase struct {
	Created string `tsv:"created"`
}

type RowWithEmbedded struct {
	EmbeddedBase
	Name string `tsv:"name"`
}

type RowWithMixedTags struct {
	Name  string `tsv:"user_name"`
	Age   int    `tsv:"user_age"`
	Email string // no tag, should use field name "Email"
}

type RowWithAllTypes struct {
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

type PtrRow struct {
	Name *string `tsv:"name"`
	Age  *int    `tsv:"age"`
}
