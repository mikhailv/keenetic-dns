package internal

import (
	"context"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type DNSResolver interface {
	Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error)
}

func NewSingleInflightDNSResolver(resolver DNSResolver) DNSResolver {
	return &singleInflightResolver{
		resolver: resolver,
		requests: map[dns.Question]*inflightRequest{},
	}
}

func NewCachedDNSResolver(resolver DNSResolver, cache *DNSCache, ttlOverride time.Duration) DNSResolver {
	return &cachedDNSResolver{
		resolver:    resolver,
		cache:       cache,
		ttlOverride: ttlOverride,
	}
}

var _ DNSResolver = (*singleInflightResolver)(nil)

type inflightRequest struct {
	Done chan struct{}
	Resp *dns.Msg
	Err  error
}

type singleInflightResolver struct {
	resolver DNSResolver
	mu       sync.Mutex
	requests map[dns.Question]*inflightRequest
}

func (s *singleInflightResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	reqKey := msg.Question[0]

	s.mu.Lock()
	if req := s.requests[reqKey]; req != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-req.Done:
			if req.Err == nil {
				resp := msg.Copy()
				resp.Response = true
				resp.Answer = req.Resp.Answer
				return resp, nil
			}
			// if we get error, then just ignore it and try to send another request
		}
		s.mu.Lock()
	}

	req := &inflightRequest{
		Done: make(chan struct{}),
	}
	s.requests[reqKey] = req
	s.mu.Unlock()

	req.Resp, req.Err = s.resolver.Resolve(ctx, msg)
	close(req.Done)

	s.mu.Lock()
	if s.requests[reqKey] == req {
		delete(s.requests, reqKey)
	}
	s.mu.Unlock()

	return req.Resp, req.Err
}

var _ DNSResolver = cachedDNSResolver{}

type cachedDNSResolver struct {
	resolver    DNSResolver
	cache       *DNSCache
	ttlOverride time.Duration
}

func (s cachedDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	if len(msg.Question) == 1 && msg.Question[0].Qtype == dns.TypeA {
		domain := msg.Question[0].Name
		if answer := s.cache.Get(domain); len(answer) > 0 {
			resp := msg.Copy()
			resp.Response = true
			resp.Answer = answer
			s.overrideTTL(resp.Answer)
			return resp, nil
		}
		resp, err := s.resolver.Resolve(ctx, msg)
		if err == nil {
			s.cache.Put(domain, resp.Answer)
			s.overrideTTL(resp.Answer)
		}
		return resp, err
	}

	return s.resolver.Resolve(ctx, msg)
}

func (s cachedDNSResolver) overrideTTL(answer []dns.RR) {
	ttlOverride := uint32(s.ttlOverride.Seconds())
	if ttlOverride == 0 {
		return
	}
	for _, rr := range answer {
		if a, ok := rr.(*dns.A); ok {
			a.Hdr.Ttl = min(a.Hdr.Ttl, ttlOverride)
		}
	}
}
