package config

import (
	"sync"
	"sync/atomic"
)

type Dynamic[T any] struct {
	data      atomic.Pointer[T]
	mu        sync.Mutex
	listeners []func()
}

func NewDynamic[T any](val T) *Dynamic[T] {
	d := &Dynamic[T]{}
	d.Set(val)
	return d
}

func (s *Dynamic[T]) Get() T {
	val := s.data.Load()
	if val == nil {
		var zero T
		return zero
	}
	return *val
}

func (s *Dynamic[T]) Set(val T) {
	s.data.Store(&val)
	s.notify()
}

func (s *Dynamic[T]) Listen(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

func (s *Dynamic[T]) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, fn := range s.listeners {
		fn()
	}
}
