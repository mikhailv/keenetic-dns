package resolvers

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/cache"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

func NewCachedResolver(name string, resolver dnssvc.Resolver, cache cache.DNSCache) dnssvc.Resolver {
	return cachedResolver{name, resolver, cache}
}

var _ dnssvc.Resolver = cachedResolver{}

type cachedResolver struct {
	name     string
	resolver dnssvc.Resolver
	cache    cache.DNSCache
}

func (s cachedResolver) Name() string {
	return s.name
}

func (s cachedResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackNamedDuration("dns_cache.resolve", s.name)()
	if dnssvc.HasSingleQuestion(req) {
		st := time.Now()
		if resp := s.cache.Get(ctx, req.Question[0]); resp != nil {
			metrics.TrackStatus("dns_cache", "hit")
			resp.SetReply(req)
			dnssvc.SetResolverInfoInContext(ctx, s.Name(), time.Since(st))
			return resp, nil
		}
		metrics.TrackStatus("dns_cache", "miss")
		resp, err := s.resolver.Resolve(ctx, req)
		// TODO: cache succeeded and failed requests separately
		if err == nil && resp.Rcode == dns.RcodeSuccess && len(resp.Answer) > 0 {
			s.cache.Put(ctx, resp)
		}
		return resp, err
	}
	return s.resolver.Resolve(ctx, req)
}

func (s cachedResolver) Close() error {
	return s.resolver.Close()
}
