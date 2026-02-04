package handlers

import (
	"context"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/cache"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

func NewCachedHandler(handler dnssvc.Handler, cache cache.DNSCache) dnssvc.Handler {
	return cachedHandler{handler, cache}
}

var _ dnssvc.Handler = cachedHandler{}

type cachedHandler struct {
	handler dnssvc.Handler
	cache   cache.DNSCache
}

func (s cachedHandler) Handle(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackDuration("dns.cache.handle")()
	if dnssvc.HasSingleQuestion(req, dns.TypeA, dns.TypeHTTPS) {
		query := req.Question[0]
		if resp := s.cache.Get(ctx, query); resp != nil {
			metrics.TrackStatus("dns.cache", "hit")
			resp.SetReply(req)
			return resp, nil
		}
		metrics.TrackStatus("dns.cache", "miss")
		resp, err := s.handler.Handle(ctx, req)
		// TODO: cache succeeded and failed requests separately
		if err == nil {
			s.cache.Put(ctx, query, resp)
		}
		return resp, err
	}
	return s.handler.Handle(ctx, req)
}
