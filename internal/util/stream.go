package util

import (
	"cmp"
	"iter" //nolint:gci // some linter bug
	"sync"
	"sync/atomic"
	"time" //nolint:gci // some linter bug
)

type Stream[T any] interface {
	Append(value T)
	Listen(listener func(cursor uint64, val T)) (stop func())
}

var _ Stream[string] = (*BufferedStream[string])(nil)

type BufferedStream[T any] struct {
	mu           sync.RWMutex
	buf          *ringBuf[streamEntry[T]]
	cursor       uint32
	listeners    map[int]func(cursor uint64, val T)
	nextListener int
}

type streamEntry[T any] struct {
	Cursor uint64
	Val    T
}

func NewBufferedStream[T any](bufferSize int) *BufferedStream[T] {
	return &BufferedStream[T]{
		buf: newRingBuf[streamEntry[T]](bufferSize),
	}
}

func (s *BufferedStream[T]) Append(value T) {
	cursor := (uint64(time.Now().UnixMilli()) << 20) | uint64(atomic.AddUint32(&s.cursor, 1)&0xfffff) // store only 52 bits
	s.mu.Lock()
	s.buf.Add(streamEntry[T]{cursor, value})
	for _, listener := range s.listeners {
		listener(cursor, value)
	}
	s.mu.Unlock()
}

func (s *BufferedStream[T]) Query(cursor uint64, count int, predicate func(val T) bool) (values []T, lastCursor uint64, hasMore bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queryLocked(false, cursor, count, predicate)
}

func (s *BufferedStream[T]) QueryBackward(cursor uint64, count int, predicate func(val T) bool) (values []T, lastCursor uint64, hasMore bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queryLocked(true, cursor, count, predicate)
}

func (s *BufferedStream[T]) queryLocked(backward bool, cursor uint64, count int, predicate func(val T) bool) (values []T, lastCursor uint64, hasMore bool) {
	var iterator iter.Seq[streamEntry[T]]
	var cmpSign int

	if backward {
		iterator = s.buf.BackwardIterator()
		cmpSign = -1
	} else {
		iterator = s.buf.Iterator()
		cmpSign = 1
	}

	lastCursor = cursor
	for it := range iterator {
		if cmp.Compare(it.Cursor, cursor) != cmpSign {
			continue
		}
		if predicate != nil && !predicate(it.Val) {
			continue
		}
		if len(values) >= count {
			hasMore = true
			break
		}
		if values == nil {
			values = make([]T, 0, count)
		}
		values = append(values, it.Val)
		lastCursor = it.Cursor
	}
	return values, lastCursor, hasMore
}

func (s *BufferedStream[T]) Listen(listener func(cursor uint64, val T)) (stop func()) {
	s.mu.Lock()
	listenerKey := s.nextListener
	s.nextListener++
	s.listeners[listenerKey] = listener
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		delete(s.listeners, listenerKey)
		s.mu.Unlock()
	}
}
