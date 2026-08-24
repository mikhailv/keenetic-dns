package handlers

import (
	"context"
	"sync"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

var _ dnssvc.Handler = &singleFlightHandler{}

func NewSingleFlightHandler(handler dnssvc.Handler) dnssvc.Handler {
	return &singleFlightHandler{
		handler:  handler,
		requests: map[dns.Question]*inflightRequest{},
	}
}

type inflightRequest struct {
	Done chan struct{}
	Resp *dns.Msg
	Err  error
	Info dnssvc.QueryInfo
	ID   types.QueryID
}

type singleFlightHandler struct {
	handler  dnssvc.Handler
	mu       sync.Mutex
	requests map[dns.Question]*inflightRequest
}

func (s *singleFlightHandler) Handle(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	if !dnssvc.HasSingleQuestion(msg) {
		return s.handler.Handle(ctx, msg)
	}

	reqKey := msg.Question[0]

	for {
		s.mu.Lock()
		if pendingReq := s.requests[reqKey]; pendingReq != nil {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-pendingReq.Done:
				if pendingReq.Err == nil {
					info := pendingReq.Info
					info.ReusedFrom = pendingReq.ID
					dnssvc.SetQueryInfo(ctx, info)
					resp := pendingReq.Resp.Copy()
					resp.SetReply(msg)
					resp.Rcode = pendingReq.Resp.Rcode
					return resp, nil
				}
				// if we get an error, then ignore it and try to send another request
			}
		} else {
			req := &inflightRequest{
				Done: make(chan struct{}),
			}
			s.requests[reqKey] = req
			s.mu.Unlock()

			req.Resp, req.Err = s.handler.Handle(ctx, msg)
			req.Info = dnssvc.GetQueryInfo(ctx)
			req.ID = ctxutil.GetDNSQueryID(ctx)
			close(req.Done)

			s.mu.Lock()
			if s.requests[reqKey] == req {
				delete(s.requests, reqKey)
			}
			s.mu.Unlock()

			return req.Resp, req.Err
		}
	}
}
