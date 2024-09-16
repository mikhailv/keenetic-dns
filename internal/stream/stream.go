package stream

type Stream[T any] interface {
	Append(value T)
	Listen(listener func(cursor Cursor, val T)) (stop func())
}
