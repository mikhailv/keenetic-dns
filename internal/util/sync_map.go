package util

import (
	"iter"
	"maps"
	"sync"
)

type SyncMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func (s *SyncMap[K, V]) Set(k K, v V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[K]V{k: v}
	} else {
		s.m[k] = v
	}
}

func (s *SyncMap[K, V]) SetIfAbsent(k K, v V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[K]V{k: v}
	} else if _, ok := s.m[k]; !ok {
		s.m[k] = v
	}
}

func (s *SyncMap[K, V]) Get(k K) (V, bool) {
	s.mu.RLock()
	v, ok := s.m[k]
	s.mu.RUnlock()
	return v, ok
}

func (s *SyncMap[K, V]) Has(k K) bool {
	s.mu.RLock()
	_, ok := s.m[k]
	s.mu.RUnlock()
	return ok
}

func (s *SyncMap[K, V]) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

func (s *SyncMap[K, V]) Clear() {
	s.mu.Lock()
	clear(s.m)
	s.mu.Unlock()
}

func (s *SyncMap[K, V]) Remove(k K) {
	s.mu.Lock()
	delete(s.m, k)
	s.mu.Unlock()
}

func (s *SyncMap[K, V]) Clone() *SyncMap[K, V] {
	return &SyncMap[K, V]{m: s.Snapshot()}
}

func (s *SyncMap[K, V]) Snapshot() map[K]V {
	s.mu.RLock()
	c := maps.Clone(s.m)
	s.mu.RUnlock()
	return c
}

func (s *SyncMap[K, V]) Iterator() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for k, v := range s.m {
			if !yield(k, v) {
				break
			}
		}
	}
}
