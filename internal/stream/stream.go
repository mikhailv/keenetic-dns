package stream

type Listener[T any] func(cursor Cursor, val T)

type Stream[T any] interface {
	Append(value T)
	Listen(listener Listener[T]) (stop func())
}
