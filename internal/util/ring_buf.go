package util

import "iter"

func newRingBuf[T any](capacity int) *ringBuf[T] {
	return &ringBuf[T]{
		end: -1,
		buf: make([]T, capacity),
	}
}

type ringBuf[T any] struct {
	start int
	end   int
	size  int
	buf   []T
}

func (s *ringBuf[T]) Add(item T) {
	capacity := cap(s.buf)
	if s.size == capacity {
		s.start = roundIndex(s.start, 1, capacity)
	} else {
		s.size++
	}
	s.end = roundIndex(s.end, 1, capacity)
	s.buf[s.end] = item
}

func (s *ringBuf[T]) Size() int {
	return s.size
}

func (s *ringBuf[T]) Slice(from, count int) []T {
	if s.size == 0 || from < 0 || count <= 0 || from >= s.size {
		return nil
	}
	capacity := cap(s.buf)
	res := make([]T, count)
	for i := 0; i < count; i++ {
		res[i] = s.buf[(from+i)%capacity]
	}
	return res
}

func (s *ringBuf[T]) Values() []T {
	return s.Slice(0, s.size)
}

func (s *ringBuf[T]) Iterator() iter.Seq[T] {
	return func(yield func(T) bool) {
		capacity := cap(s.buf)
		for i := 0; i < s.size; i++ {
			if !yield(s.buf[roundIndex(s.start, i, capacity)]) {
				break
			}
		}
	}
}

func (s *ringBuf[T]) BackwardIterator() iter.Seq[T] {
	return func(yield func(T) bool) {
		capacity := cap(s.buf)
		for i := 0; i < s.size; i++ {
			if !yield(s.buf[roundIndex(s.end, capacity-i, capacity)]) {
				break
			}
		}
	}
}

func roundIndex(i, d, c int) int {
	return (i + d) % c
}
