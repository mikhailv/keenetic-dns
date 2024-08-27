package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	dnsResolveBufferSize = 5_000
)

var _ DNSResolver = (*DNSRoutingService)(nil)

type DNSRoutingService struct {
	config        *RoutingConfig
	resolver      DNSResolver
	ipRoutes      *IPRouteController
	resolveStream *util.BufferedStream[DomainResolve]
}

func NewDNSRoutingService(config *RoutingConfig, resolver DNSResolver, ipRoutes *IPRouteController) *DNSRoutingService {
	return &DNSRoutingService{
		config:        config,
		resolver:      resolver,
		ipRoutes:      ipRoutes,
		resolveStream: util.NewBufferedStream[DomainResolve](dnsResolveBufferSize),
	}
}

func (s *DNSRoutingService) ResolveStream() *util.BufferedStream[DomainResolve] {
	return s.resolveStream
}

func (s *DNSRoutingService) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resp, err := s.resolver.Resolve(ctx, msg)
	if err != nil {
		return nil, err
	}

	if resp.Question[0].Qtype == dns.TypeA {
		now := time.Now()
		res := DomainResolve{
			Time:   now,
			Domain: strings.TrimRight(resp.Question[0].Name, "."),
			A:      make([]ARecord, 0, len(resp.Answer)),
		}
		for _, it := range resp.Answer {
			if a, ok := it.(*dns.A); ok {
				res.A = append(res.A, ARecord{NewIPv4(a.A), int(a.Hdr.Ttl)})
			}
		}
		s.resolveStream.Append(res)

		if iface := s.config.LookupHost(res.Domain); iface != "" {
			for _, it := range res.A {
				s.ipRoutes.AddRoute(ctx, NewDNSRecord(res.Domain, it.IP, now.Add(time.Duration(it.TTL)*time.Second)), iface)
			}
		}

		fmt.Printf("\t%s\t%d ip\n", res.Domain, len(res.A))
	}

	return resp, nil
}
