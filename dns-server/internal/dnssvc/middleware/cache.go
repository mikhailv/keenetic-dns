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
		return cachedResolver{handler, cache}.Resolve
	}
}

type cachedResolver struct {
	handler dnssvc.Handler
	cache   QueryResultCache
}

func (s cachedResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackDuration("dns.cache.handle")()
	if dnssvc.HasSingleQuestion(req, dns.TypeA, dns.TypeHTTPS) {
		query := req.Question[0]
		if resp := s.cache.Get(query); resp != nil {
			metrics.TrackStatus("dns.cache", "hit")
			resp.SetReply(req)
			return resp, nil
		}
		metrics.TrackStatus("dns.cache", "miss")
		resp, err := s.handler(ctx, req)
		// TODO: cache succeeded and failed requests separately
		if err == nil {
			s.cache.Put(query, resp)
		}
		return resp, err
	}
	return s.handler(ctx, req)
}
