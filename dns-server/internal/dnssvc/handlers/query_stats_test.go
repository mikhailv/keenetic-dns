package handlers

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/blocklist"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type markingUpstream struct {
	mark func(ctx context.Context)
	err  error
}

func (s markingUpstream) Handle(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if s.mark != nil {
		s.mark(ctx)
	}
	if s.err != nil {
		return nil, s.err
	}
	resp := new(dns.Msg)
	resp.SetReply(req)
	return resp, nil
}

func TestQueryStats_RecordsEveryQuery(t *testing.T) {
	recorder := &stubRecorder{}
	h := NewQueryStatsHandler(markingUpstream{}, recorder)

	ctx := ctxutil.WithDNSQueryClientIP(t.Context(), types.MustParseIPv4("192.168.1.5"))
	_, err := h.Handle(ctx, query("example.com", dns.TypeA))
	require.NoError(t, err)

	require.Len(t, recorder.records, 1)
	assert.Equal(t, recordedBlock{types.MustParseIPv4("192.168.1.5"), "example.com", "A", ""}, recorder.records[0])
}

func TestQueryStats_Labels(t *testing.T) {
	tests := map[string]struct {
		mark func(ctx context.Context)
		want string
	}{
		"plain":   {nil, ""},
		"blocked": {dnssvc.SetQueryBlocked, LabelBlocked},
		"routed":  {dnssvc.SetQueryRouted, LabelRouted},
		"blocked wins": {func(ctx context.Context) {
			dnssvc.SetQueryRouted(ctx)
			dnssvc.SetQueryBlocked(ctx)
		}, LabelBlocked},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := &stubRecorder{}
			h := NewQueryStatsHandler(markingUpstream{mark: tt.mark}, recorder)

			_, err := h.Handle(t.Context(), query("example.com", dns.TypeA))
			require.NoError(t, err)

			require.Len(t, recorder.records, 1)
			assert.Equal(t, tt.want, recorder.records[0].label)
		})
	}
}

func TestQueryStats_RecordsFailedQuery(t *testing.T) {
	recorder := &stubRecorder{}
	h := NewQueryStatsHandler(markingUpstream{err: errors.New("upstream down")}, recorder)

	_, err := h.Handle(t.Context(), query("example.com", dns.TypeA))
	require.Error(t, err)
	assert.Len(t, recorder.records, 1, "a query that failed upstream still happened")
}

func TestQueryStats_IgnoresMultiQuestion(t *testing.T) {
	recorder := &stubRecorder{}
	h := NewQueryStatsHandler(markingUpstream{}, recorder)

	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("a.example.com"), dns.TypeA)
	req.Question = append(req.Question, dns.Question{Name: dns.Fqdn("b.example.com"), Qtype: dns.TypeA, Qclass: dns.ClassINET})

	_, err := h.Handle(t.Context(), req)
	require.NoError(t, err)
	assert.Empty(t, recorder.records)
}

func TestQueryStats_BlockedQueryIsLabelled(t *testing.T) {
	recorder := &stubRecorder{}
	blocking := NewBlockingHandler(
		&stubUpstream{},
		blockedDomains("ads.example.com"),
		blocklist.ModeNXDomain,
		nil,
		slog.New(slog.DiscardHandler),
	)
	h := NewQueryStatsHandler(blocking, recorder)

	_, err := h.Handle(t.Context(), query("ads.example.com", dns.TypeA))
	require.NoError(t, err)

	require.Len(t, recorder.records, 1)
	assert.Equal(t, LabelBlocked, recorder.records[0].label, "the blocking handler below reports through the context")
}
