package util

import (
	"iter"
)

func NewRingBuf[T any](capacity int) *RingBuf[T] {
	return &RingBuf[T]{
		buf: make([]T, capacity),
	}
}

type RingBuf[T any] struct {
	start int
	size  int
	buf   []T
}

func (s *RingBuf[T]) Add(item T) {
	capacity := cap(s.buf)
	if s.size == capacity {
		s.buf[s.start] = item
		s.start = (s.start + 1) % capacity
	} else {
		s.buf[(s.start+s.size)%cap(s.buf)] = item
		s.size++
	}
}

func (s *RingBuf[T]) Get(i int) T {
	return s.buf[(s.start+i)%cap(s.buf)]
}

func (s *RingBuf[T]) Capacity() int {
	return cap(s.buf)
}

func (s *RingBuf[T]) Size() int {
	return s.size
}

func (s *RingBuf[T]) Slice(from, count int) []T {
	if s.size == 0 || from < 0 || count <= 0 || from >= s.size {
		return nil
	}
	capacity := cap(s.buf)
	count = min(count, s.size)
	res := make([]T, count)
	from += s.start
	for i := 0; i < count; i++ {
		res[i] = s.buf[(from+i)%capacity]
	}
	return res
}

func (s *RingBuf[T]) Values() []T {
	return s.Slice(0, s.size)
}

func (s *RingBuf[T]) Clear() {
	var zero T
	capacity := cap(s.buf)
	for i := range s.size {
		s.buf[(s.start+i)%capacity] = zero
	}
	s.start = 0
	s.size = 0
}

func (s *RingBuf[T]) Iterator(from, step int) iter.Seq[T] {
	if step == 0 {
		step = 1
	}
	return func(yield func(T) bool) {
		capacity := cap(s.buf)
		for i := from; i >= 0 && i < s.size; i += step {
			if !yield(s.buf[(s.start+i)%capacity]) {
				break
			}
		}
	}
}
