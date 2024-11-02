package stream

import (
	"cmp"
	"iter"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

var _ Stream[string] = (*Buffered[string])(nil)

type Buffered[T any] struct {
	mu           sync.RWMutex
	buf          *util.RingBuf[streamEntry[T]]
	index        int32
	listeners    map[uint16]func(cursor Cursor, val T)
	nextListener uint16
}

type QueryResult[T any] struct {
	Items       []T    `json:"items"`
	FirstCursor Cursor `json:"firstCursor"`
	LastCursor  Cursor `json:"lastCursor"`
	HasMore     bool   `json:"hasMore"`
}

func (s *QueryResult[T]) Reverse() {
	slices.Reverse(s.Items)
	s.FirstCursor, s.LastCursor = s.LastCursor, s.FirstCursor
}

type streamEntry[T any] struct {
	Cursor Cursor
	Val    T
}

func NewBufferedStream[T any](bufferSize int) *Buffered[T] {
	return &Buffered[T]{
		buf:       util.NewRingBuf[streamEntry[T]](bufferSize),
		listeners: map[uint16]func(cursor Cursor, val T){},
	}
}

func (s *Buffered[T]) Append(value T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cursor := Cursor((uint64(time.Now().UnixMilli()) << 32) | uint64(s.index))
	s.index++
	if c, ok := any(value).(CursorAware); ok {
		c.SetCursor(cursor)
	} else if c, ok := any(&value).(CursorAware); ok {
		c.SetCursor(cursor)
	}
	s.buf.Add(streamEntry[T]{cursor, value})
	for _, listener := range s.listeners {
		listener(cursor, value)
	}
}

func (s *Buffered[T]) Query(cursor Cursor, count int, predicate func(val T) bool) QueryResult[T] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query(true, cursor, count, predicate)
}

func (s *Buffered[T]) QueryBackward(cursor Cursor, count int, predicate func(val T) bool) QueryResult[T] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query(false, cursor, count, predicate)
}

func (s *Buffered[T]) lookupPos(cursor Cursor) (i int, found bool) {
	return sort.Find(s.buf.Size(), func(i int) int {
		return cmp.Compare(cursor, s.buf.Get(i).Cursor)
	})
}

//nolint:cyclop // readable enough
func (s *Buffered[T]) query(forward bool, cursor Cursor, count int, predicate func(val T) bool) QueryResult[T] {
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

func (s *Buffered[T]) Listen(listener func(cursor Cursor, val T)) (stop func()) {
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
