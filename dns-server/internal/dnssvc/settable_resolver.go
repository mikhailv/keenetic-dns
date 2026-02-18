package dnssvc

import (
	"context"
	"sync"

	"github.com/miekg/dns"
)

type SettableResolver struct {
	mu       sync.RWMutex
	resolver Resolver
}

func NewSettableResolver(resolver Resolver) *SettableResolver {
	return &SettableResolver{resolver: resolver}
}

func (s *SettableResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resolver.Resolve(ctx, msg)
}

func (s *SettableResolver) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolver.Close()
}

func (s *SettableResolver) SetResolver(resolver Resolver) error {
	s.mu.Lock()
	err := s.resolver.Close()
	s.resolver = resolver
	s.mu.Unlock()
	return err
}
