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
	if dnssvc.HasSingleQuestion(req) {
		if resp := s.cache.Get(ctx, req.Question[0]); resp != nil {
			metrics.TrackStatus("dns.cache", "hit")
			resp.SetReply(req)
			return resp, nil
		}
		metrics.TrackStatus("dns.cache", "miss")
		resp, err := s.handler.Handle(ctx, req)
		// TODO: cache succeeded and failed requests separately
		if err == nil && resp.Rcode == dns.RcodeSuccess && len(resp.Answer) > 0 {
			s.cache.Put(ctx, resp)
		}
		return resp, err
	}
	return s.handler.Handle(ctx, req)
}
