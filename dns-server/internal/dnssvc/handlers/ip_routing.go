package handlers

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
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

func NewIPRoutingHandler(
	handler dnssvc.Handler,
	dnsStore *storage.DNSStore,
	ipRoutes *routing.IPRouteController,
	stream stream.Stream[types.DNSQuery],
	logger *slog.Logger,
) dnssvc.Handler {
	return &ipRoutingHandler{handler, dnsStore, ipRoutes, stream, logger}
}

var _ dnssvc.Handler = (*ipRoutingHandler)(nil)

type ipRoutingHandler struct {
	handler  dnssvc.Handler
	dnsStore *storage.DNSStore
	ipRoutes *routing.IPRouteController
	stream   stream.Stream[types.DNSQuery]
	logger   *slog.Logger
}

func (s *ipRoutingHandler) Handle(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resolveTime := types.TimestampFromTime(time.Now())
	ctx = dnssvc.WithResolvedByContext(ctx)
	resp, err := s.handler.Handle(ctx, msg)
	if err == nil && dnssvc.HasSingleQuestion(msg, dns.TypeA) {
		s.processTypeAResponse(ctx, resp, resolveTime, dnssvc.GetResolvedBy(ctx))
	}
	return resp, err
}

func (s *ipRoutingHandler) processTypeAResponse(ctx context.Context, resp *dns.Msg, resolveTime types.Timestamp, resolvedBy types.ResolvedBy) {
	domain := resp.Question[0].Name
	dl := s.parseResponse(domain, resp.Answer)
	dl.Time = resolveTime
	dl.ResolvedBy = resolvedBy
	if len(dl.IPs) > 0 {
		s.resolveReverseRecords(ctx, dl)
		s.processDomainLookup(ctx, dl)
	}
}

func (s *ipRoutingHandler) parseResponse(domain string, answers []dns.RR) *types.DomainLookup {
	res := &types.DomainLookup{
		Domain: domain,
		IPs:    make([]types.DomainIP, 0, 10),
	}
	for _, rr := range answers {
		if v, ok := rr.(*dns.A); ok && v.Hdr.Name == domain {
			res.IPs = append(res.IPs, types.DomainIP{
				IP:  types.NewIPv4(v.A),
				TTL: v.Hdr.Ttl,
			})
		}
	}
	if len(res.IPs) > 0 {
		return res
	}

	ipsByName := map[string][]dns.A{}
	cnameByName := map[string]dns.CNAME{}

	for _, rr := range answers {
		switch v := rr.(type) {
		case *dns.A:
			ipsByName[v.Hdr.Name] = append(ipsByName[v.Hdr.Name], *v)
		case *dns.CNAME:
			cnameByName[v.Hdr.Name] = *v
		}
	}

	var visited util.Set[string]

	name := domain
	for !visited.Has(name) {
		if cn, ok := cnameByName[name]; ok {
			res.CNames = append(res.CNames, types.DomainEntry[string]{
				Name: cn.Target,
				TTL:  cn.Hdr.Ttl,
			})
			visited.Add(name)
			name = cn.Target
			continue
		}
		for _, it := range ipsByName[name] {
			res.IPs = append(res.IPs, types.DomainIP{
				IP:  types.NewIPv4(it.A),
				TTL: it.Hdr.Ttl,
			})
		}
		break
	}

	return res
}

func (s *ipRoutingHandler) resolveReverseRecords(ctx context.Context, dl *types.DomainLookup) {
	if len(dl.IPs) == 0 {
		return
	}
	if len(dl.IPs) == 1 {
		s.reverseLookup(ctx, &dl.IPs[0])
		return
	}
	var wg sync.WaitGroup
	for i := range dl.IPs {
		wg.Go(func() {
			s.reverseLookup(ctx, &dl.IPs[i])
		})
	}
	wg.Wait()
}

func (s *ipRoutingHandler) reverseLookup(ctx context.Context, dip *types.DomainIP) {
	ip := dip.IP
	if ip.HasPrefix() { // network address, should not be
		return
	}

	var sb strings.Builder
	sb.Grow(32)
	for i := 3; i >= 0; i-- {
		sb.WriteString(strconv.Itoa(int(ip[i])))
		sb.WriteByte('.')
	}
	sb.WriteString("in-addr.arpa.")
	domain := sb.String()

	req := &dns.Msg{}
	req.SetQuestion(domain, dns.TypePTR)
	req.RecursionDesired = true

	if resp, err := s.handler.Handle(ctx, req); err != nil {
		s.logger.Error("failed PTR request", "err", err, "domain", domain, "ip", ip.String())
	} else {
		for _, it := range resp.Answer {
			if v, ok := it.(*dns.PTR); ok {
				dip.PTR = append(dip.PTR, types.DomainEntry[string]{
					Name: v.Ptr,
					TTL:  v.Hdr.Ttl,
				})
			}
		}
		for _, it := range resp.Ns {
			if v, ok := it.(*dns.SOA); ok {
				dip.SOA = append(dip.SOA, types.DomainEntry[string]{
					Name: v.Ns,
					TTL:  v.Hdr.Ttl,
				})
			}
		}
		if len(dip.PTR) == 0 && len(dip.SOA) == 0 {
			s.logger.Warn("unexpected PTR response without PTR/SOA data", "domain", domain, "ip", ip.String())
		}
	}
}

func (s *ipRoutingHandler) processDomainLookup(ctx context.Context, dl *types.DomainLookup) {
	slices.SortFunc(dl.IPs, func(a, b types.DomainIP) int {
		return bytes.Compare(a.IP[:], b.IP[:])
	})

	s.dnsStore.Add(dl)

	routedIPs := s.ipRoutes.AddRoutes(ctx, dl)

	s.stream.Append(types.DNSQuery{
		ClientAddr:   ctxutil.GetDNSQueryRemoteAddr(ctx),
		DomainLookup: *dl,
		Duration:     time.Since(dl.Time.Time()).Seconds(),
		RoutedIPs:    routedIPs,
	})
}
