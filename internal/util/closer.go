package util

import "io"

var _ io.Closer = CloserFunc(nil)

type CloserFunc func() error

func (c CloserFunc) Close() error {
	return c()
}
