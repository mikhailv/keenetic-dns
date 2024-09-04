package util

import "sync"

type SyncSet[T comparable] struct {
	mu      sync.RWMutex
	entries map[T]struct{}
}

func NewSyncSet[T comparable]() *SyncSet[T] {
	return &SyncSet[T]{entries: map[T]struct{}{}}
}

func (s *SyncSet[T]) Add(v T) bool {
	s.mu.Lock()
	if _, ok := s.entries[v]; ok {
		s.mu.Unlock()
		return false
	}
	s.entries[v] = struct{}{}
	s.mu.Unlock()
	return true
}

func (s *SyncSet[T]) Has(v T) bool {
	s.mu.RLock()
	_, ok := s.entries[v]
	s.mu.RUnlock()
	return ok
}

func (s *SyncSet[T]) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *SyncSet[T]) Clear() {
	s.mu.Lock()
	clear(s.entries)
	s.mu.Unlock()
}

func (s *SyncSet[T]) Remove(v T) {
	s.mu.Lock()
	delete(s.entries, v)
	s.mu.Unlock()
}

func (s *SyncSet[T]) Iterate(fn func(v T) bool) {
	s.mu.RLock()
	for v := range s.entries {
		if !fn(v) {
			break
		}
	}
	s.mu.RUnlock()
}
