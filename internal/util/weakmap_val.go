package util

import (
	"weak"
)

// WeakMapVal is WeakMap implementation for value types like slices, maps and channels
type WeakMapVal[K comparable, V any] struct {
	baseWeakMap[K, V]
}

func (s *WeakMapVal[K, V]) Get(k K) *V {
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

func (s *WeakMapVal[K, V]) GetOrCompute(k K, compute func() V) V {
	// lookup entry with read-lock
	s.mu.RLock()
	p, ok := s.m[k]
	s.mu.RUnlock()
	if ok {
		if v := p.Value(); v != nil {
			return *v
		}
	}
	// acquire write-lock and lookup entry again
	s.mu.Lock()
	p, ok = s.m[k]
	if ok {
		if v := p.Value(); v != nil {
			s.mu.Unlock()
			return *v
		}
	}
	s.cleanupOnMiss()
	// compute value and store
	v := compute()
	if s.m == nil {
		s.m = map[K]weak.Pointer[V]{}
	}
	s.m[k] = weak.Make(&v)
	s.mu.Unlock()
	return v
}

func (s *WeakMapVal[K, V]) Put(k K, v V) {
	s.mu.Lock()
	if s.m == nil {
		s.m = map[K]weak.Pointer[V]{}
	}
	s.m[k] = weak.Make(&v)
	s.mu.Unlock()
}
