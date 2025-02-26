package resolver

import (
	"context"
	"time"

	"github.com/miekg/dns"
)

func NewTTLOverridingDNSResolver(resolver DNSResolver, ttl time.Duration) DNSResolver {
	if ttl <= 0 {
		return resolver
	}
	return ttlOverridingDNSResolver{resolver, ttl}
}

var _ DNSResolver = ttlOverridingDNSResolver{}

type ttlOverridingDNSResolver struct {
	resolver DNSResolver
	ttl      time.Duration
}

func (s ttlOverridingDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resp, err := s.resolver.Resolve(ctx, msg)
	if err != nil {
		return nil, err
	}
	if HasSingleQuestion(msg, dns.TypeA) {
		ttlOverride := uint32(s.ttl.Seconds())
		if ttlOverride > 0 {
			for _, rr := range resp.Answer {
				if a, ok := rr.(*dns.A); ok {
					a.Hdr.Ttl = min(a.Hdr.Ttl, ttlOverride)
				}
			}
		}
	}
	return resp, nil
}
