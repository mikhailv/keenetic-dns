package middleware

import (
	"context"
	"sync"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

var _ dnssvc.Middleware = SingleInflightMiddleware

func SingleInflightMiddleware(handler dnssvc.Handler) dnssvc.Handler {
	return newSingleInflightDNSResolver(handler).Resolve
}

func newSingleInflightDNSResolver(handler dnssvc.Handler) *singleInflightResolver {
	return &singleInflightResolver{
		handler:  handler,
		requests: map[dns.Question]*inflightRequest{},
	}
}

type inflightRequest struct {
	Done chan struct{}
	Resp *dns.Msg
	Err  error
}

type singleInflightResolver struct {
	handler  dnssvc.Handler
	mu       sync.Mutex
	requests map[dns.Question]*inflightRequest
}

func (s *singleInflightResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	if !dnssvc.HasSingleQuestion(msg) {
		return s.handler(ctx, msg)
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

	req.Resp, req.Err = s.handler(ctx, msg)
	close(req.Done)

	s.mu.Lock()
	if s.requests[reqKey] == req {
		delete(s.requests, reqKey)
	}
	s.mu.Unlock()

	return req.Resp, req.Err
}
