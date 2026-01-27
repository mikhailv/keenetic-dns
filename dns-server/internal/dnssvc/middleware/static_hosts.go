package middleware

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

func NewStaticHostsMiddleware(hosts *config.Dynamic[config.Hosts], ttl time.Duration) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		return staticHostsMiddleware{handler, hosts, ttl}.Resolve
	}
}

type staticHostsMiddleware struct {
	handler dnssvc.Handler
	hosts   *config.Dynamic[config.Hosts]
	ttl     time.Duration
}

func (s staticHostsMiddleware) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if dnssvc.HasSingleQuestion(req, dns.TypeA) {
		domain := req.Question[0].Name
		if host, ok := s.hosts.Get()[domain]; ok {
			resp := &dns.Msg{}
			resp.SetRcode(req, dns.RcodeSuccess)
			resp.Answer = []dns.RR{&dns.A{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    uint32(s.ttl.Seconds()),
				},
				A: host.IP,
			}}
			return resp, nil
		}
	}
	return s.handler(ctx, req)
}
