package handlers

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

func NewTTLOverrideHandler(handler dnssvc.Handler, ttl time.Duration) dnssvc.Handler {
	return ttlOverrideHandler{handler, ttl}
}

var _ dnssvc.Handler = ttlOverrideHandler{}

type ttlOverrideHandler struct {
	handler dnssvc.Handler
	ttl     time.Duration
}

func (s ttlOverrideHandler) Handle(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	resp, err := s.handler.Handle(ctx, req)
	if err != nil {
		return nil, err
	}
	ttlOverride := uint32(s.ttl.Seconds())
	if ttlOverride > 0 {
		for _, rr := range resp.Answer {
			rr.Header().Ttl = min(rr.Header().Ttl, ttlOverride)
		}
	}
	return resp, nil
}
