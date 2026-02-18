package handlers

import (
	"bytes"
	"context"
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

func NewIPRoutingHandler(
	handler dnssvc.Handler,
	dnsStore *storage.DNSStore,
	ipRoutes *routing.IPRouteController,
	stream stream.Stream[types.DNSQuery],
) dnssvc.Handler {
	return ipRoutingHandler{handler, dnsStore, ipRoutes, stream}
}

var _ dnssvc.Handler = ipRoutingHandler{}

type ipRoutingHandler struct {
	handler  dnssvc.Handler
	dnsStore *storage.DNSStore
	ipRoutes *routing.IPRouteController
	stream   stream.Stream[types.DNSQuery]
}

func (s ipRoutingHandler) Handle(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resp, err := s.handler.Handle(ctx, msg)
	if err == nil && dnssvc.HasSingleQuestion(msg, dns.TypeA) {
		s.processTypeAResponse(ctx, resp)
	}
	return resp, err
}

func (s ipRoutingHandler) processTypeAResponse(ctx context.Context, resp *dns.Msg) {
	reqName := resp.Question[0].Name
	ipsByName, cnameByName, ttl := s.extractAnswerRecords(resp.Answer)
	ips := s.resolveIPs(reqName, ipsByName, cnameByName, &ttl)
	if len(ips) > 0 {
		s.processResolvedIPs(ctx, reqName, ttl, ips)
	}
}

// extractAnswerRecords parses DNS answer section into A records (grouped by name), CNAME records, and minimum TTL.
func (s ipRoutingHandler) extractAnswerRecords(answers []dns.RR) (map[string][]types.IPv4, map[string]dns.CNAME, uint32) {
	ipsByName := map[string][]types.IPv4{}
	var cnameByName map[string]dns.CNAME
	minTTL := uint32(math.MaxUint32)
	for _, rr := range answers {
		switch v := rr.(type) {
		case *dns.A:
			ipsByName[v.Hdr.Name] = append(ipsByName[v.Hdr.Name], types.NewIPv4(v.A))
			minTTL = min(minTTL, v.Hdr.Ttl)
		case *dns.CNAME:
			if cnameByName == nil {
				cnameByName = map[string]dns.CNAME{}
			}
			cnameByName[v.Hdr.Name] = *v
		}
	}
	return ipsByName, cnameByName, minTTL
}

// resolveIPs follows CNAME chain from reqName to find final IPs and determines routing for each.
func (s ipRoutingHandler) resolveIPs(reqName string, ipsByName map[string][]types.IPv4, cnameByName map[string]dns.CNAME, ttl *uint32) []types.ResolvedIP {
	var visited util.Set[string]
	var routedByName bool
	var routingIface, routingPattern string

	name := reqName
	for !visited.Has(name) {
		if !routedByName {
			routedByName, routingPattern, routingIface = s.ipRoutes.LookupHost(name)
		}
		cn, ok := cnameByName[name]
		if !ok {
			return s.buildResolvedIPs(ipsByName[name], routedByName, routingIface, routingPattern)
		}
		visited.Add(name)
		name = cn.Target
		*ttl = min(*ttl, cn.Hdr.Ttl)
	}
	return nil
}

func (s ipRoutingHandler) buildResolvedIPs(ips []types.IPv4, routedByName bool, routingIface, routingPattern string) []types.ResolvedIP {
	result := make([]types.ResolvedIP, len(ips))
	for i, ip := range ips {
		staticAddress, _, _ := s.ipRoutes.LookupIP(ip)
		result[i] = types.ResolvedIP{
			IP:          ip,
			RouteAdded:  routedByName && !staticAddress,
			RouteIface:  routingIface,
			RouteReason: routingPattern,
		}
	}
	return result
}

func (s ipRoutingHandler) processResolvedIPs(ctx context.Context, domain string, ttl uint32, ips []types.ResolvedIP) {
	for i, it := range ips {
		if it.RouteIface == "" {
			if ok, pattern, iface := s.ipRoutes.LookupIP(it.IP); ok {
				it.RouteIface = iface
				it.RouteReason = pattern
				ips[i] = it
			}
		}
	}

	slices.SortFunc(ips, func(a, b types.ResolvedIP) int {
		return bytes.Compare(a.IP[:], b.IP[:])
	})

	res := types.DNSQuery{
		Time:       time.Now(),
		ClientAddr: ctxutil.GetDNSQueryRemoteAddr(ctx),
		Domain:     domain,
		TTL:        max(ttl, 1),
		IPs:        ips,
	}
	for i := range res.IPs {
		it := &res.IPs[i]
		s.dnsStore.Add(types.NewDNSRecord(res.Domain, it.IP, res.Time, int(res.TTL)))
		if it.RouteAdded {
			it.RouteAdded = s.ipRoutes.AddRoute(ctx, it.IP, "routed: "+it.RouteReason)
		}
	}

	s.stream.Append(res)
}
