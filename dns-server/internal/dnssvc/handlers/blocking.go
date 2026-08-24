package handlers

import (
	"context"
	"log/slog"
	"net"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/blocklist"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

const blockedTTL = 60

type Blocklist interface {
	Mode() blocklist.Mode
	Lookup(domain string, clientIP types.IPv4) (blocklist.Match, bool)
}

type StatsRecorder interface {
	Record(clientIP types.IPv4, domain, qtype, label string)
}

func NewBlockingHandler(
	handler dnssvc.Handler,
	list Blocklist,
	recorder StatsRecorder,
	logger *slog.Logger,
) dnssvc.Handler {
	return blockingHandler{handler: handler, list: list, recorder: recorder, logger: logger}
}

var _ dnssvc.Handler = blockingHandler{}

type blockingHandler struct {
	handler  dnssvc.Handler
	list     Blocklist
	recorder StatsRecorder
	logger   *slog.Logger
}

func (s blockingHandler) Handle(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if !dnssvc.HasSingleQuestion(req) {
		return s.handler.Handle(ctx, req)
	}
	question := req.Question[0]
	domain := question.Name
	clientIP := ctxutil.GetDNSQueryClientIP(ctx)

	if match, ok := s.blocked(domain, clientIP); ok {
		return s.block(ctx, req, domain, domain, match, clientIP), nil
	}

	resp, err := s.handler.Handle(ctx, req)
	if err != nil || resp == nil {
		return resp, err
	}

	if target, match, ok := s.blockedCNAME(resp, clientIP); ok {
		s.logger.Debug("blocked cname target", "domain", domain, "target", target, "list", match.List)
		return s.block(ctx, req, domain, target, match, clientIP), nil
	}
	return resp, nil
}

func (s blockingHandler) blocked(domain string, clientIP types.IPv4) (blocklist.Match, bool) {
	match, ok := s.list.Lookup(domain, clientIP)
	if !ok || !match.Blocked() {
		return blocklist.Match{}, false
	}
	return match, true
}

func (s blockingHandler) blockedCNAME(resp *dns.Msg, clientIP types.IPv4) (string, blocklist.Match, bool) {
	for _, rr := range resp.Answer {
		cname, ok := rr.(*dns.CNAME)
		if !ok {
			continue
		}
		if match, ok := s.blocked(cname.Target, clientIP); ok {
			return cname.Target, match, true
		}
	}
	return "", blocklist.Match{}, false
}

func (s blockingHandler) block(
	ctx context.Context,
	req *dns.Msg,
	domain string,
	blockedDomain string,
	match blocklist.Match,
	clientIP types.IPv4,
) *dns.Msg {
	qtype := dns.TypeToString[req.Question[0].Qtype]

	dnssvc.SetQueryBlocked(ctx)
	dnssvc.SetQueryBlockInfo(ctx, types.BlockInfo{
		List:    match.List,
		Domain:  blockedDomain,
		Pattern: match.Pattern,
	})
	if s.recorder != nil {
		s.recorder.Record(clientIP, domain, qtype, match.List)
	}
	s.logger.Debug("query blocked", "domain", domain, "blocked_domain", blockedDomain, "qtype", qtype, "list", match.List)
	return s.response(req)
}

func (s blockingHandler) response(req *dns.Msg) *dns.Msg {
	resp := &dns.Msg{}
	switch s.list.Mode() {
	case blocklist.ModeNXDomain:
		resp.SetRcode(req, dns.RcodeNameError)
	case blocklist.ModeNull:
		resp.SetReply(req)
		if rr := nullRecord(req.Question[0]); rr != nil {
			resp.Answer = append(resp.Answer, rr)
		}
	case blocklist.ModeNoData:
		resp.SetReply(req)
	default:
		resp.SetRcode(req, dns.RcodeNameError)
	}
	resp.Authoritative = true
	return resp
}

func nullRecord(question dns.Question) dns.RR {
	header := dns.RR_Header{
		Name:   question.Name,
		Rrtype: question.Qtype,
		Class:  dns.ClassINET,
		Ttl:    blockedTTL,
	}
	switch question.Qtype {
	case dns.TypeA:
		return &dns.A{Hdr: header, A: net.IPv4zero}
	case dns.TypeAAAA:
		return &dns.AAAA{Hdr: header, AAAA: net.IPv6zero}
	default:
		return nil
	}
}
