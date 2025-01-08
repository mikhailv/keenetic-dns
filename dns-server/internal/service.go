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
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	dnsQueryHistorySize = 5_000
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
		queryStream: stream.NewBufferedStream[DNSQuery](dnsQueryHistorySize),
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

	if hasSingleQuestion(msg, dns.TypeA) {
		s.processTypeAResponse(ctx, resp)
	}

	return resp, nil
}

func (s *DNSRoutingService) processTypeAResponse(ctx context.Context, resp *dns.Msg) { //nolint:cyclop // ignore for now
	reqName := resp.Question[0].Name

	var cnames util.LazyMap[string, dns.CNAME]
	for _, rr := range resp.Answer {
		if cn, ok := rr.(*dns.CNAME); ok {
			cnames.Set(cn.Hdr.Name, *cn)
		}
	}

	var nameIPs util.LazyMap[string, []IPv4]
	var ttl uint32 = math.MaxUint32
	for _, rr := range resp.Answer {
		if a, ok := rr.(*dns.A); ok {
			nameIPs.Set(a.Hdr.Name, append(nameIPs[a.Hdr.Name], NewIPv4(a.A)))
			ttl = min(ttl, a.Hdr.Ttl)
		}
	}

	var ips []IPv4
	var ifaces util.Set[string]
	var aliases util.Set[string]

	for name := reqName; !aliases.Has(name); {
		aliases.Add(name)
		if iface := s.ipRoutes.LookupHost(normalizeName(name)); iface != "" {
			ifaces.Add(iface)
		}
		if cn, ok := cnames[name]; ok {
			name = cn.Target
			ttl = min(ttl, cn.Hdr.Ttl)
		} else {
			ips = nameIPs[name]
			break
		}
	}

	if len(ips) > 0 {
		slices.SortFunc(ips, func(a, b IPv4) int {
			return bytes.Compare(a[:], b[:])
		})
		res := DNSQuery{
			Time:   time.Now(),
			Domain: normalizeName(reqName),
			TTL:    max(ttl, 1),
			IPs:    ips,
			Routed: ifaces.Values(),
		}
		s.queryStream.Append(res)
		for _, ip := range res.IPs {
			s.dnsStore.Add(NewDNSRecord(res.Domain, ip, res.Time.Add(time.Duration(res.TTL)*time.Second)))
			for _, iface := range res.Routed {
				s.ipRoutes.AddRoute(ctx, iface, ip)
			}
		}
		s.logger.Debug("domain resolved", "domain", res.Domain, "ips", len(res.IPs))
	}
}

func normalizeName(name string) string {
	return strings.TrimRight(name, ".")
}
