package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/blocklist"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type stubBlocklist struct {
	matches map[string]blocklist.Match
	mode    blocklist.Mode
}

func (s stubBlocklist) Lookup(domain string, _ types.IPv4) (blocklist.Match, bool) {
	match, ok := s.matches[strings.TrimSuffix(domain, ".")]
	return match, ok
}

func (s stubBlocklist) Mode() blocklist.Mode {
	return s.mode
}

func blockedDomains(domains ...string) stubBlocklist {
	s := stubBlocklist{matches: map[string]blocklist.Match{}}
	for _, d := range domains {
		s.matches[d] = blocklist.Match{List: "test", Action: blocklist.Block}
	}
	return s
}

type stubUpstream struct {
	called bool
	answer []dns.RR
	err    error
}

func (s *stubUpstream) Handle(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Answer = s.answer
	return resp, nil
}

type recordedBlock struct {
	clientIP types.IPv4
	domain   string
	qtype    string
	label    string
}

type stubRecorder struct{ records []recordedBlock }

func (s *stubRecorder) Record(clientIP types.IPv4, domain, qtype, label string) {
	s.records = append(s.records, recordedBlock{clientIP, domain, qtype, label})
}

func query(domain string, qtype uint16) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(domain), qtype)
	return req
}

func newHandler(t *testing.T, list stubBlocklist, mode blocklist.Mode) (dnssvc.Handler, *stubUpstream, *stubRecorder) {
	t.Helper()
	list.mode = mode
	upstream := &stubUpstream{}
	recorder := &stubRecorder{}
	h := NewBlockingHandler(upstream, list, recorder, slog.New(slog.DiscardHandler))
	return h, upstream, recorder
}

func TestBlocking_BlockedQueryNeverReachesUpstream(t *testing.T) {
	h, upstream, _ := newHandler(t, blockedDomains("ads.example.com"), blocklist.ModeNXDomain)

	resp, err := h.Handle(t.Context(), query("ads.example.com", dns.TypeA))
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeNameError, resp.Rcode)
	assert.False(t, upstream.called, "a blocked query must not be forwarded")
}

func TestBlocking_AllowedQueryPassesThrough(t *testing.T) {
	h, upstream, recorder := newHandler(t, blockedDomains("ads.example.com"), blocklist.ModeNXDomain)

	_, err := h.Handle(t.Context(), query("example.org", dns.TypeA))
	require.NoError(t, err)
	assert.True(t, upstream.called)
	assert.Empty(t, recorder.records)
}

func TestBlocking_Modes(t *testing.T) {
	tests := []struct {
		name   string
		mode   blocklist.Mode
		qtype  uint16
		rcode  int
		answer string // expected address, empty when no answer is expected
	}{
		{name: "nxdomain", mode: blocklist.ModeNXDomain, qtype: dns.TypeA, rcode: dns.RcodeNameError},
		{name: "nodata", mode: blocklist.ModeNoData, qtype: dns.TypeA, rcode: dns.RcodeSuccess},
		{name: "null a", mode: blocklist.ModeNull, qtype: dns.TypeA, rcode: dns.RcodeSuccess, answer: "0.0.0.0"},
		{name: "null aaaa", mode: blocklist.ModeNull, qtype: dns.TypeAAAA, rcode: dns.RcodeSuccess, answer: "::"},
		{name: "null other type", mode: blocklist.ModeNull, qtype: dns.TypeMX, rcode: dns.RcodeSuccess},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _ := newHandler(t, blockedDomains("ads.example.com"), tt.mode)

			resp, err := h.Handle(t.Context(), query("ads.example.com", tt.qtype))
			require.NoError(t, err)
			assert.Equal(t, tt.rcode, resp.Rcode)

			if tt.answer == "" {
				assert.Empty(t, resp.Answer)
				return
			}
			require.Len(t, resp.Answer, 1)
			var got net.IP
			switch rr := resp.Answer[0].(type) {
			case *dns.A:
				got = rr.A
			case *dns.AAAA:
				got = rr.AAAA
			default:
				t.Fatalf("unexpected record type %T", rr)
			}
			assert.Equal(t, tt.answer, got.String())
			assert.Equal(t, uint32(blockedTTL), resp.Answer[0].Header().Ttl)
		})
	}
}

func TestBlocking_CNAMECloaking(t *testing.T) {
	h, upstream, recorder := newHandler(t, blockedDomains("tracker.example.net"), blocklist.ModeNXDomain)
	upstream.answer = []dns.RR{
		&dns.CNAME{
			Hdr:    dns.RR_Header{Name: "metrics.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
			Target: "tracker.example.net.",
		},
	}

	resp, err := h.Handle(t.Context(), query("metrics.example.com", dns.TypeA))
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeNameError, resp.Rcode)
	require.Len(t, recorder.records, 1)
	assert.Equal(t, "metrics.example.com.", recorder.records[0].domain,
		"stats are keyed by the domain the client asked for, not by the cname it resolved to")
	assert.Equal(t, "test", recorder.records[0].label)
}

func TestBlocking_CleanCNAMEPassesThrough(t *testing.T) {
	h, upstream, _ := newHandler(t, blockedDomains("tracker.example.net"), blocklist.ModeNXDomain)
	upstream.answer = []dns.RR{
		&dns.CNAME{
			Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
			Target: "cdn.example.com.",
		},
	}

	resp, err := h.Handle(t.Context(), query("www.example.com", dns.TypeA))
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeSuccess, resp.Rcode)
	assert.Len(t, resp.Answer, 1)
}

func TestBlocking_AllowRuleIsNotBlocked(t *testing.T) {
	list := stubBlocklist{matches: map[string]blocklist.Match{
		"cdn.example.com": {List: "test", Action: blocklist.Allow},
	}}
	h, upstream, recorder := newHandler(t, list, blocklist.ModeNXDomain)

	_, err := h.Handle(t.Context(), query("cdn.example.com", dns.TypeA))
	require.NoError(t, err)
	assert.True(t, upstream.called)
	assert.Empty(t, recorder.records)
}

func TestBlocking_RecordsClientAndQuestion(t *testing.T) {
	h, _, recorder := newHandler(t, blockedDomains("ads.example.com"), blocklist.ModeNXDomain)
	ctx := ctxutil.WithDNSQueryClientIP(t.Context(), types.MustParseIPv4("192.168.1.42"))

	_, err := h.Handle(ctx, query("ads.example.com", dns.TypeAAAA))
	require.NoError(t, err)

	require.Len(t, recorder.records, 1)
	assert.Equal(t, "192.168.1.42", recorder.records[0].clientIP.String())
	assert.Equal(t, "ads.example.com.", recorder.records[0].domain)
	assert.Equal(t, "AAAA", recorder.records[0].qtype)
	assert.Equal(t, "test", recorder.records[0].label)
}

func TestBlocking_UpstreamErrorPropagates(t *testing.T) {
	h, upstream, _ := newHandler(t, blockedDomains("ads.example.com"), blocklist.ModeNXDomain)
	upstream.err = errors.New("upstream unavailable")

	_, err := h.Handle(t.Context(), query("example.org", dns.TypeA))
	assert.Error(t, err)
}

func TestBlocking_NonSingleQuestionPassesThrough(t *testing.T) {
	h, upstream, _ := newHandler(t, blockedDomains("ads.example.com"), blocklist.ModeNXDomain)

	req := new(dns.Msg)
	req.Question = []dns.Question{
		{Name: "ads.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		{Name: "example.org.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
	}

	_, err := h.Handle(t.Context(), req)
	require.NoError(t, err)
	assert.True(t, upstream.called)
}

func TestBlocking_NilRecorderIsSafe(t *testing.T) {
	upstream := &stubUpstream{}
	h := NewBlockingHandler(upstream, blockedDomains("ads.example.com"), nil, slog.New(slog.DiscardHandler))

	resp, err := h.Handle(t.Context(), query("ads.example.com", dns.TypeA))
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeNameError, resp.Rcode)
}
