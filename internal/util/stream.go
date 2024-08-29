package util

import (
	"cmp"
	"iter" //nolint:gci // some linter bug
	"slices"
	"sort"
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

type QueryResult[T any] struct {
	Items       []T    `json:"items"`
	FirstCursor uint64 `json:"firstCursor"`
	LastCursor  uint64 `json:"lastCursor"`
	HasMore     bool   `json:"hasMore"`
}

func (s *QueryResult[T]) Reverse() {
	slices.Reverse(s.Items)
	s.FirstCursor, s.LastCursor = s.LastCursor, s.FirstCursor
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

func (s *BufferedStream[T]) Query(cursor uint64, count int, predicate func(val T) bool) QueryResult[T] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query(true, cursor, count, predicate)
}

func (s *BufferedStream[T]) QueryBackward(cursor uint64, count int, predicate func(val T) bool) QueryResult[T] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query(false, cursor, count, predicate)
}

func (s *BufferedStream[T]) lookupPos(cursor uint64) (i int, found bool) {
	return sort.Find(s.buf.Size(), func(i int) int {
		return cmp.Compare(cursor, s.buf.Get(i).Cursor)
	})
}

func (s *BufferedStream[T]) query(forward bool, cursor uint64, count int, predicate func(val T) bool) QueryResult[T] {
	backward := !forward

	pos, found := s.lookupPos(cursor)
	if backward {
		pos--
	} else if found && forward {
		pos++
	}

	res := QueryResult[T]{
		FirstCursor: cursor,
		LastCursor:  cursor,
	}

	if pos < 0 || pos >= s.buf.Size() {
		return res
	}

	var iterator iter.Seq[streamEntry[T]]
	if forward {
		iterator = s.buf.Iterator(pos, 1)
	} else {
		iterator = s.buf.Iterator(pos, -1)
	}

	res.LastCursor = cursor
	res.FirstCursor = cursor
	for it := range iterator {
		if predicate != nil && !predicate(it.Val) {
			continue
		}
		if len(res.Items) >= count {
			res.HasMore = true
			break
		}
		if res.Items == nil {
			res.Items = make([]T, 0, count)
		}
		res.Items = append(res.Items, it.Val)
		if len(res.Items) == 1 {
			res.FirstCursor = it.Cursor
		}
		res.LastCursor = it.Cursor
	}
	return res
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
