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
	lastCursor   Cursor
	listeners    map[uint16]Listener[T]
	nextListener uint16
}

type QueryResult[T any] struct {
	Items       []T    `json:"items"`
	FirstCursor Cursor `json:"first_cursor"`
	LastCursor  Cursor `json:"last_cursor"`
	HasMore     bool   `json:"has_more"`
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
		listeners: map[uint16]Listener[T]{},
	}
}

func (s *Buffered[T]) Append(value T) {
	s.mu.Lock()
	cursor := NewCursor(time.Now(), 0)
	if cursor <= s.lastCursor {
		cursor = s.lastCursor + 1
	}
	s.lastCursor = cursor
	if c, ok := any(value).(CursorAware); ok {
		c.SetCursor(cursor)
	} else if c, ok := any(&value).(CursorAware); ok {
		c.SetCursor(cursor)
	}
	s.buf.Add(streamEntry[T]{cursor, value})
	listeners := s.listenersCopy()
	s.mu.Unlock()
	for _, listener := range listeners {
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

func (s *Buffered[T]) listenersCopy() []Listener[T] {
	res := make([]Listener[T], 0, len(s.listeners))
	for _, fn := range s.listeners {
		res = append(res, fn)
	}
	return res
}

func (s *Buffered[T]) lookupPos(cursor Cursor) (i int, found bool) {
	return sort.Find(s.buf.Size(), func(i int) int {
		return cmp.Compare(cursor, s.buf.Get(i).Cursor)
	})
}

func (s *Buffered[T]) query(forward bool, cursor Cursor, count int, predicate func(val T) bool) QueryResult[T] {
	res := QueryResult[T]{
		FirstCursor: cursor,
		LastCursor:  cursor,
	}

	if s.buf.Size() == 0 {
		return res
	}

	pos, found := s.lookupPos(cursor)
	if found {
		if forward {
			pos++
		} else {
			pos--
		}
	} else {
		if forward {
			pos = 0
		} else {
			pos = s.buf.Size() - 1
		}
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

func (s *Buffered[T]) Listen(listener Listener[T]) (stop func()) {
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
