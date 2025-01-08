package internal

import (
	"context"
	"fmt"
	"slices"
	"strings"
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

func NewCachedDNSResolver(resolver DNSResolver, cache *DNSCache) DNSResolver {
	return &cachedDNSResolver{
		resolver: resolver,
		cache:    cache,
	}
}

func NewTTLOverridingDNSResolver(resolver DNSResolver, ttl time.Duration) DNSResolver {
	if ttl <= 0 {
		return resolver
	}
	return ttlOverridingDNSResolver{resolver, ttl}
}

func NewVerboseDNSResolver(resolver DNSResolver) DNSResolver {
	return verboseDNSResolver{resolver}
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
	if !hasSingleQuestion(msg) {
		return s.resolver.Resolve(ctx, msg)
	}

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
	resolver DNSResolver
	cache    *DNSCache
}

func (s cachedDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	if hasSingleQuestion(msg, dns.TypeA) {
		query := msg.Question[0]
		if resp := s.cache.Get(query); resp != nil {
			resp.Id = msg.Id
			return resp, nil
		}
		resp, err := s.resolver.Resolve(ctx, msg)
		if err == nil {
			s.cache.Put(query, resp)
		}
		return resp, err
	}

	return s.resolver.Resolve(ctx, msg)
}

var _ DNSResolver = ttlOverridingDNSResolver{}

type ttlOverridingDNSResolver struct {
	resolver DNSResolver
	ttl      time.Duration
}

func (s ttlOverridingDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resp, err := s.resolver.Resolve(ctx, msg)
	if err != nil {
		return nil, err
	}
	if hasSingleQuestion(msg, dns.TypeA) {
		ttlOverride := uint32(s.ttl.Seconds())
		if ttlOverride > 0 {
			for _, rr := range resp.Answer {
				if a, ok := rr.(*dns.A); ok {
					a.Hdr.Ttl = min(a.Hdr.Ttl, ttlOverride)
				}
			}
		}
	}
	return resp, nil
}

var _ DNSResolver = verboseDNSResolver{}

type verboseDNSResolver struct {
	resolver DNSResolver
}

func (s verboseDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	fmt.Println(">> DNS:\n" + indent(msg.Question[0].String()))
	resp, err := s.resolver.Resolve(ctx, msg)
	if err != nil {
		fmt.Println("<< DNS error: " + err.Error())
	} else {
		fmt.Println("<< DNS:\n" + indent(resp.String()))
	}
	return resp, err
}

func indent(text string) string {
	return "\t" + strings.Join(strings.Split(text, "\n"), "\n\t")
}

func hasSingleQuestion(msg *dns.Msg, types ...uint16) bool {
	if len(msg.Question) != 1 {
		return false
	}
	return len(types) == 0 || slices.Contains(types, msg.Question[0].Qtype)
}
