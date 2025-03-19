package middleware

import (
	"context"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

type QueryResultCache interface {
	Get(query dns.Question) *dns.Msg
	Put(query dns.Question, result *dns.Msg)
}

func NewCacheMiddleware(cache QueryResultCache) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		return func(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
			return handleRequestCaching(ctx, req, handler, cache)
		}
	}
}

func handleRequestCaching(ctx context.Context, req *dns.Msg, handler dnssvc.Handler, cache QueryResultCache) (*dns.Msg, error) {
	defer metrics.TrackDuration("dns.cache.handle")()
	if dnssvc.HasSingleQuestion(req, dns.TypeA, dns.TypeHTTPS) {
		query := req.Question[0]
		if resp := cache.Get(query); resp != nil {
			metrics.TrackStatus("dns.cache", "hit")
			resp.Id = req.Id
			return resp, nil
		}
		metrics.TrackStatus("dns.cache", "miss")
		resp, err := handler(ctx, req)
		// TODO: cache succeeded and failed requests separately
		if err == nil {
			cache.Put(query, resp)
		}
		return resp, err
	}
	return handler(ctx, req)
}
