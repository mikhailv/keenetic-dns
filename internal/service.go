package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

var _ DNSResolver = dnsRoutingService{}

type dnsRoutingService struct {
	config   *RoutingConfig
	resolver DNSResolver
	ipRoutes *IPRouteController
}

func NewDNSRoutingService(config *RoutingConfig, resolver DNSResolver, ipRoutes *IPRouteController) DNSResolver {
	return dnsRoutingService{
		config:   config,
		resolver: resolver,
		ipRoutes: ipRoutes,
	}
}

func (s dnsRoutingService) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resp, err := s.resolver.Resolve(ctx, msg)
	if err != nil {
		return nil, err
	}

	if resp.Question[0].Qtype == dns.TypeA {
		domain := strings.TrimRight(resp.Question[0].Name, ".")
		if iface, ok := s.config.LookupHost(domain); ok {
			for _, it := range resp.Answer {
				if a, ok := it.(*dns.A); ok {
					rec := NewDNSRecord(domain, NewIPv4(a.A), time.Duration(a.Hdr.Ttl)*time.Second)
					s.ipRoutes.AddRoute(ctx, rec, iface)
				}
			}
		}
		fmt.Printf("\t%s\t%d ip\n", domain, len(resp.Answer))
	}

	return resp, nil
}
