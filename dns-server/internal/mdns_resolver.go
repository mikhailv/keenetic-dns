package internal

import (
	"context"
	"strings"

	"github.com/miekg/dns"
)

var _ DNSResolver = mdnsResolver{}

type mdnsResolver struct {
	cfg      MDNSConfig
	client   *mdnsClient
	resolver DNSResolver
}

func NewMDNSResolver(resolver DNSResolver, cfg MDNSConfig) DNSResolver {
	return mdnsResolver{
		cfg:      cfg,
		client:   newMDNSClient(cfg.Addr, cfg.QueryTimeout),
		resolver: resolver,
	}
}

func (s mdnsResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	if s.shouldProcessQuery(msg) {
		return s.client.Resolve(ctx, msg)
	}
	return s.resolver.Resolve(ctx, msg)
}

func (s mdnsResolver) shouldProcessQuery(msg *dns.Msg) bool {
	if hasSingleQuestion(msg, dns.TypeA) {
		for _, domain := range s.cfg.Domains {
			if strings.HasSuffix(msg.Question[0].Name, domain) {
				return true
			}
		}
	}
	return false
}
