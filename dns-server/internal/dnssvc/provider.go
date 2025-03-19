package dnssvc

import (
	"fmt"
	"slices"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
)

type Provider interface {
	Resolver
	MatchQuery(msg *dns.Msg) int32
}

type provider struct {
	Resolver
	cfg   config.DNSProvider
	types []uint16
}

func NewProvider(resolver Resolver, cfg config.DNSProvider) Provider {
	return &provider{
		Resolver: resolver,
		cfg:      cfg,
		types:    parseQueryTypes(cfg.Types),
	}
}

func (s *provider) MatchQuery(msg *dns.Msg) int32 {
	if !HasSingleQuestion(msg, s.types...) {
		return -1
	}
	domain := msg.Question[0].Name
	if s.cfg.Ignore.Match(domain) > 0 {
		return -1
	}
	priority := byte(max(0, min(255, s.cfg.Priority)))
	score := s.cfg.Domains.Match(domain)
	if score < 0 {
		return -1
	}
	return int32(score)<<8 | int32(priority)
}

func parseQueryTypes(types []string) []uint16 {
	r := make([]uint16, len(types))
	for i, t := range types {
		switch t {
		case "A":
			r[i] = dns.TypeA
		case "AAAA":
			r[i] = dns.TypeAAAA
		case "CNAME":
			r[i] = dns.TypeCNAME
		case "HTTPS":
			r[i] = dns.TypeHTTPS
		default:
			panic(fmt.Sprintf("unsupported query type %q", t))
		}
	}
	slices.Sort(r)
	return slices.Compact(r)
}
