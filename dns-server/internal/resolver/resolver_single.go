package resolver

import (
	"context"
	"sync"

	"github.com/miekg/dns"
)

func NewSingleInflightDNSResolver(resolver DNSResolver) DNSResolver {
	return &singleInflightResolver{
		resolver: resolver,
		requests: map[dns.Question]*inflightRequest{},
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
	if !HasSingleQuestion(msg) {
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
				resp := &dns.Msg{}
				resp.SetRcode(msg, req.Resp.Rcode)
				resp.Answer = req.Resp.Answer
				resp.Ns = req.Resp.Ns
				resp.Extra = req.Resp.Extra
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
