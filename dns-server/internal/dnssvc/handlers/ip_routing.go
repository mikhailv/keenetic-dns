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

	var cnameByName util.LazyMap[string, dns.CNAME]
	var ipsByName util.LazyMap[string, []types.IPv4]

	var ttl uint32 = math.MaxUint32

	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *dns.A:
			ipsByName.Set(v.Hdr.Name, append(ipsByName[v.Hdr.Name], types.NewIPv4(v.A)))
			ttl = min(ttl, v.Hdr.Ttl)
		case *dns.CNAME:
			cnameByName.Set(v.Hdr.Name, *v)
		}
	}

	var ips []types.ResolvedIP
	var visited util.Set[string]

	var routedByName bool
	var routingIface string
	var routingPattern string

	for name := reqName; !visited.Has(name); {
		if !routedByName {
			routedByName, routingPattern, routingIface = s.ipRoutes.LookupHost(name)
		}
		if cn, ok := cnameByName[name]; ok {
			visited.Add(name)
			name = cn.Target
			ttl = min(ttl, cn.Hdr.Ttl)
		} else {
			nameIPs := ipsByName[name]
			ips = make([]types.ResolvedIP, len(nameIPs))
			for i, ip := range nameIPs {
				staticAddress, _, _ := s.ipRoutes.LookupIP(ip)
				ips[i] = types.ResolvedIP{
					IP:          ip,
					RouteAdded:  routedByName && !staticAddress,
					RouteIface:  routingIface,
					RouteReason: routingPattern,
				}
			}
			break
		}
	}

	if len(ips) > 0 {
		s.processResolvedIPs(ctx, reqName, ttl, ips)
	}
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
