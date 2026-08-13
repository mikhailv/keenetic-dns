package dnssvc

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
)

type QueryMatchResult struct {
	Score   int32
	Pattern string
}

type Provider interface {
	Resolver
	MatchQuery(msg *dns.Msg) QueryMatchResult
}

type provider struct {
	Resolver
	cfg   config.DNSProvider
	types []uint16
}

func NewProvider(resolver Resolver, cfg config.DNSProvider) (Provider, error) {
	types, err := parseQueryTypes(cfg.Types)
	if err != nil {
		return nil, err
	}
	return &provider{resolver, cfg, types}, nil
}

func (s *provider) MatchQuery(msg *dns.Msg) QueryMatchResult {
	if !HasSingleQuestion(msg, s.types...) {
		return QueryMatchResult{Score: -1}
	}
	domain := msg.Question[0].Name
	ignorePattern := s.cfg.Ignore.Match(domain)
	if ignorePattern != "" {
		return QueryMatchResult{Score: -1, Pattern: ignorePattern}
	}
	priority := byte(max(0, min(255, s.cfg.Priority)))
	if s.cfg.Domains.Empty() {
		return QueryMatchResult{Score: int32(priority)}
	}
	pattern := s.cfg.Domains.Match(domain)
	if pattern == "" {
		return QueryMatchResult{Score: -1}
	}
	return QueryMatchResult{
		Score:   int32(len(pattern))<<8 | int32(priority),
		Pattern: pattern,
	}
}

func (s *provider) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	if len(s.cfg.Rewrite) > 0 {
		domain := msg.Question[0].Name
		for fromSuffix, toSuffix := range s.cfg.Rewrite {
			if before, ok := strings.CutSuffix(domain, fromSuffix); ok {
				return s.rewriteResolve(ctx, msg.Copy(), domain, before+toSuffix)
			}
		}
	}
	return s.Resolver.Resolve(ctx, msg)
}

func (s *provider) rewriteResolve(ctx context.Context, msg *dns.Msg, fromDomain, toDomain string) (*dns.Msg, error) {
	msg.Question[0].Name = toDomain
	resp, err := s.Resolver.Resolve(ctx, msg)
	if err != nil {
		return nil, err
	}
	resp.Question[0].Name = fromDomain
	for _, rr := range resp.Answer {
		if rr.Header().Name == toDomain {
			rr.Header().Name = fromDomain
		}
	}
	return resp, nil
}

func parseQueryTypes(types []string) ([]uint16, error) {
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
		case "PTR":
			r[i] = dns.TypePTR
		default:
			return nil, fmt.Errorf("unsupported query type %q", t)
		}
	}
	slices.Sort(r)
	return slices.Compact(r), nil
}
