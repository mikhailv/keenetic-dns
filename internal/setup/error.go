package setup

import "fmt"

func ExitIfError(err error) {
	if err != nil {
		ExitWithError(err)
	}
}

func ExitWithError(err error) {
	panic(fmt.Errorf("setup: %w", err))
}

func UnwrapOrExit[T any](v T, err error) T {
	ExitIfError(err)
	return v
}
