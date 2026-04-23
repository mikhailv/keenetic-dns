package setup

import (
	"fmt"
	"os"
)

func ExitIfError(err error) {
	if err != nil {
		ExitWithError(err)
	}
}

func ExitWithError(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func Unwrap[T any](v T, err error) T {
	ExitIfError(err)
	return v
}
