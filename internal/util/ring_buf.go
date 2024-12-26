package util

import "iter"

func NewRingBuf[T any](capacity int) *RingBuf[T] {
	return &RingBuf[T]{
		end: -1,
		buf: make([]T, capacity),
	}
}

type RingBuf[T any] struct {
	start int
	end   int
	size  int
	buf   []T
}

func (s *RingBuf[T]) Add(item T) {
	capacity := cap(s.buf)
	if s.size == capacity {
		s.start = (s.start + 1) % capacity
	} else {
		s.size++
	}
	s.end = (s.end + 1) % capacity
	s.buf[s.end] = item
}

func (s *RingBuf[T]) Get(i int) T {
	return s.buf[(s.start+i)%cap(s.buf)]
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
