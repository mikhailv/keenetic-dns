package util

import (
	"sync"
	"sync/atomic"
	"weak"
)

const weakMapCleanupMissesThreshold = 100

type WeakMap[K comparable, V any] struct {
	baseWeakMap[K, V]
}

func (s *WeakMap[K, V]) Get(k K) *V {
	s.mu.RLock()
	p, ok := s.m[k]
	s.mu.RUnlock()
	if ok {
		if r := p.Value(); r != nil {
			return r
		}
		s.cleanupOnMiss()
	}
	return nil
}

func (s *WeakMap[K, V]) GetOrCompute(k K, compute func() *V) *V {
	// lookup entry with read-lock
	s.mu.RLock()
	p, ok := s.m[k]
	s.mu.RUnlock()
	if ok {
		if v := p.Value(); v != nil {
			return v
		}
	}
	// acquire write-lock and lookup entry again
	s.mu.Lock()
	p, ok = s.m[k]
	if ok {
		if v := p.Value(); v != nil {
			s.mu.Unlock()
			return v
		}
	}
	s.cleanupOnMiss()
	// compute value and store
	v := compute()
	if s.m == nil {
		s.m = map[K]weak.Pointer[V]{}
	}
	s.m[k] = weak.Make(v)
	s.mu.Unlock()
	return v
}

func (s *WeakMap[K, V]) Put(k K, v *V) {
	s.mu.Lock()
	if s.m == nil {
		s.m = map[K]weak.Pointer[V]{}
	}
	s.m[k] = weak.Make(v)
	s.mu.Unlock()
}

type baseWeakMap[K comparable, T any] struct {
	mu     sync.RWMutex
	m      map[K]weak.Pointer[T]
	misses atomic.Int32
}

func (s *baseWeakMap[K, V]) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

func (s *baseWeakMap[K, V]) Remove(k K) {
	s.mu.Lock()
	delete(s.m, k)
	s.mu.Unlock()
}

func (s *baseWeakMap[K, V]) cleanupOnMiss() {
	if s.misses.Add(1) == weakMapCleanupMissesThreshold {
		go s.cleanup()
	}
}

func (s *baseWeakMap[K, V]) cleanup() {
	s.mu.Lock()
	for k, v := range s.m {
		if v.Value() == nil {
			delete(s.m, k)
		}
	}
	s.mu.Unlock()
	// reset counter after cleanup
	s.misses.Store(0)
}
