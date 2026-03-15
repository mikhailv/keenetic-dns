package util

func UnwrapResult[T any](v T, err error) T {
	PanicIf(err)
	return v
}

func PanicIf(err error) {
	if err != nil {
		panic(err)
	}
}
