package resolver

import (
	"context"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/cache"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

func NewCachedDNSResolver(resolver DNSResolver, cache *cache.DNSCache) DNSResolver {
	return &cachedDNSResolver{
		resolver: resolver,
		cache:    cache,
	}
}

var _ DNSResolver = cachedDNSResolver{}

type cachedDNSResolver struct {
	resolver DNSResolver
	cache    *cache.DNSCache
}

func (s cachedDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackDuration("dns.cache.handle")()
	if HasSingleQuestion(msg, dns.TypeA) {
		query := msg.Question[0]
		if resp := s.cache.Get(query); resp != nil {
			metrics.TrackStatus("dns.cache", "hit")
			resp.Id = msg.Id
			return resp, nil
		}
		metrics.TrackStatus("dns.cache", "miss")
		resp, err := s.resolver.Resolve(ctx, msg)
		// TODO: cache succeeded and failed requests separately
		if err == nil {
			s.cache.Put(query, resp)
		}
		return resp, err
	}

	return s.resolver.Resolve(ctx, msg)
}
