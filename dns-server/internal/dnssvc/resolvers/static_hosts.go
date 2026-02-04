package resolvers

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

func NewStaticHostResolver(hosts config.Hosts, ttl time.Duration) dnssvc.Resolver {
	return staticHostResolver{hosts, ttl}
}

var _ dnssvc.Resolver = staticHostResolver{}

type staticHostResolver struct {
	hosts config.Hosts
	ttl   time.Duration
}

func (s staticHostResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if dnssvc.HasSingleQuestion(req, dns.TypeA) {
		domain := req.Question[0].Name
		if ip, ok := s.hosts[domain]; ok {
			resp := &dns.Msg{}
			resp.SetRcode(req, dns.RcodeSuccess)
			resp.Answer = []dns.RR{&dns.A{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    uint32(s.ttl.Seconds()),
				},
				A: ip,
			}}
			return resp, nil
		}
	}
	return dnssvc.RefusedResponse(req), nil
}

func (s staticHostResolver) Close() error {
	return nil
}
