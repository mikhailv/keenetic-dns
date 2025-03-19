package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/routing"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/storage"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

var _ dnssvc.Resolver = (*DNSRoutingService)(nil)

type DNSRoutingService struct {
	logger         *slog.Logger
	resolver       dnssvc.Resolver
	dnsStore       *storage.DNSStore
	ipRoutes       *routing.IPRouteController
	queryStream    *stream.Buffered[types.DNSQuery]
	rawQueryStream *stream.Buffered[types.DNSRawQuery]
}

func NewDNSRoutingService(
	logger *slog.Logger,
	resolver dnssvc.Resolver,
	dnsStore *storage.DNSStore,
	ipRoutes *routing.IPRouteController,
	queryStream *stream.Buffered[types.DNSQuery],
	rawQueryStream *stream.Buffered[types.DNSRawQuery],
) *DNSRoutingService {
	return &DNSRoutingService{
		logger:         logger,
		resolver:       resolver,
		dnsStore:       dnsStore,
		ipRoutes:       ipRoutes,
		queryStream:    queryStream,
		rawQueryStream: rawQueryStream,
	}
}

func (s *DNSRoutingService) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	s.appendRawQuery(ctx, false, msg.String())

	resp, err := s.resolver.Resolve(ctx, msg)
	if err != nil {
		s.appendRawQuery(ctx, true, fmt.Sprintf("ERROR: query (id: %d) failed: %v", msg.Id, err))
		return nil, err
	}
	s.appendRawQuery(ctx, true, resp.String())

	if dnssvc.HasSingleQuestion(msg, dns.TypeA) {
		s.processTypeAResponse(ctx, resp)
	}

	return resp, nil
}

func (s *DNSRoutingService) appendRawQuery(ctx context.Context, response bool, text string) {
	s.rawQueryStream.Append(types.DNSRawQuery{
		Time:       time.Now(),
		ClientAddr: ctxutil.GetDNSQueryRemoteAddr(ctx),
		Response:   response,
		Text:       text,
	})
}

func (s *DNSRoutingService) processTypeAResponse(ctx context.Context, resp *dns.Msg) {
	reqName := resp.Question[0].Name

	var cnames util.LazyMap[string, dns.CNAME]
	var nameIPs util.LazyMap[string, []types.IPv4]

	var ttl uint32 = math.MaxUint32

	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *dns.A:
			nameIPs.Set(v.Hdr.Name, append(nameIPs[v.Hdr.Name], types.NewIPv4(v.A)))
			ttl = min(ttl, v.Hdr.Ttl)
		case *dns.CNAME:
			cnames.Set(v.Hdr.Name, *v)
		}
	}

	var ips []types.IPv4
	var ifaces util.Set[string]
	var visited util.Set[string]

	for name := reqName; !visited.Has(name); {
		if iface := s.ipRoutes.LookupHost(name); iface != "" {
			ifaces.Add(iface)
		}
		if cn, ok := cnames[name]; ok {
			visited.Add(name)
			name = cn.Target
			ttl = min(ttl, cn.Hdr.Ttl)
		} else {
			ips = nameIPs[name]
			break
		}
	}

	if len(ips) > 0 {
		slices.SortFunc(ips, func(a, b types.IPv4) int {
			return bytes.Compare(a[:], b[:])
		})
		res := types.DNSQuery{
			Time:       time.Now(),
			ClientAddr: ctxutil.GetDNSQueryRemoteAddr(ctx),
			Domain:     reqName,
			TTL:        max(ttl, 1),
			IPs:        ips,
			Routed:     ifaces.Values(),
		}
		s.queryStream.Append(res)
		for _, ip := range res.IPs {
			s.dnsStore.Add(types.NewDNSRecord(res.Domain, ip, res.Time.Add(time.Duration(res.TTL)*time.Second)))
			for _, iface := range res.Routed {
				s.ipRoutes.AddRoute(ctx, iface, ip)
			}
		}
		s.logger.Debug("domain resolved", "domain", res.Domain, "ips", len(res.IPs), "client_addr", res.ClientAddr)
	}
}
