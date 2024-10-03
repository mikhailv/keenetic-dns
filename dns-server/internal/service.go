package internal

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/internal/stream"
)

const (
	dnsResolveBufferSize = 5_000
)

var _ DNSResolver = (*DNSRoutingService)(nil)

type DNSRoutingService struct {
	logger      *slog.Logger
	resolver    DNSResolver
	dnsStore    *DNSStore
	ipRoutes    *IPRouteController
	queryStream *stream.Buffered[DNSQuery]
}

func NewDNSRoutingService(logger *slog.Logger, resolver DNSResolver, dnsStore *DNSStore, ipRoutes *IPRouteController) *DNSRoutingService {
	return &DNSRoutingService{
		logger:      logger,
		resolver:    resolver,
		dnsStore:    dnsStore,
		ipRoutes:    ipRoutes,
		queryStream: stream.NewBufferedStream[DNSQuery](dnsResolveBufferSize),
	}
}

func (s *DNSRoutingService) QueryStream() *stream.Buffered[DNSQuery] {
	return s.queryStream
}

func (s *DNSRoutingService) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resp, err := s.resolver.Resolve(ctx, msg)
	if err != nil {
		return nil, err
	}

	if resp.Question[0].Qtype == dns.TypeA {
		now := time.Now()
		res := DNSQuery{
			Time:   now,
			Domain: strings.TrimRight(resp.Question[0].Name, "."),
			TTL:    math.MaxUint32,
			IPs:    make([]IPv4, 0, len(resp.Answer)),
		}
		for _, it := range resp.Answer {
			if a, ok := it.(*dns.A); ok {
				res.TTL = min(res.TTL, a.Hdr.Ttl)
				res.IPs = append(res.IPs, NewIPv4(a.A))
			}
		}
		slices.SortFunc(res.IPs, func(a, b IPv4) int {
			return bytes.Compare(a[:], b[:])
		})

		if len(res.IPs) > 0 {
			s.queryStream.Append(res)

			if iface := s.ipRoutes.LookupHost(res.Domain); iface != "" {
				for _, ip := range res.IPs {
					s.dnsStore.Add(NewDNSRecord(res.Domain, ip, now.Add(time.Duration(res.TTL)*time.Second)))
					s.ipRoutes.AddRoute(ctx, iface, ip)
				}
			}

			s.logger.Debug("domain resolved", "domain", res.Domain, "ips", len(res.IPs))
		}
	}

	return resp, nil
}
