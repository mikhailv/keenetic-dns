package handlers

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
)

type observingHandler struct {
	domain string
	calls  atomic.Int32
	block  chan struct{} // when set, the handler waits on it, so callers pile up behind one in-flight request
}

func (h *observingHandler) Handle(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	h.calls.Add(1)
	if h.block != nil {
		<-h.block
	}
	dnssvc.SetQueryObservation(ctx, dnssvc.QueryObservation{
		Lookup: &types.DomainLookup{Domain: h.domain, IPs: []types.DomainIP{{IP: types.MustParseIPv4("1.2.3.4")}}},
	})
	resp := &dns.Msg{}
	resp.SetReply(msg)
	return resp, nil
}

func queryFor(domain string) *dns.Msg {
	msg := &dns.Msg{}
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	return msg
}

func TestQueryLog_CoalescedQueriesAllReported(t *testing.T) {
	const clients = 5

	release := make(chan struct{})
	upstream := &observingHandler{domain: "example.com.", block: release}
	queries := stream.NewBufferedStream[types.DNSQuery](100)

	handler := NewQueryLogHandler(NewSingleInflightHandler(upstream), queries)

	var wg sync.WaitGroup
	for i := range clients {
		wg.Go(func() {
			ctx := ctxutil.WithDNSQueryClientIP(t.Context(), types.MustParseIPv4("192.168.1."+string(rune('1'+i))))
			_, err := handler.Handle(ctx, queryFor("example.com"))
			assert.NoError(t, err)
		})
	}

	require.Eventually(t, func() bool { return upstream.calls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	res := queries.Query(0, 100, nil)
	assert.Equal(t, int32(1), upstream.calls.Load(), "the exchange must still happen once")
	require.Len(t, res.Items, clients, "every client must be reported")

	seen := map[types.IPv4]bool{}
	for _, q := range res.Items {
		assert.Equal(t, "example.com.", q.Domain)
		seen[q.ClientIP] = true
	}
	assert.Len(t, seen, clients, "each entry must carry the address of the client that asked")
}

func TestQueryLog_ReportsResolvedQuery(t *testing.T) {
	upstream := &observingHandler{domain: "example.com."}
	queries := stream.NewBufferedStream[types.DNSQuery](10)
	handler := NewQueryLogHandler(upstream, queries)

	client := types.MustParseIPv4("192.168.1.10")
	ctx := ctxutil.WithDNSQueryClientIP(t.Context(), client)
	_, err := handler.Handle(ctx, queryFor("example.com"))
	require.NoError(t, err)

	res := queries.Query(0, 10, nil)
	require.Len(t, res.Items, 1)
	assert.Equal(t, client, res.Items[0].ClientIP)
	assert.Equal(t, "example.com.", res.Items[0].Domain)
}

func TestQueryLog_NothingObservedNothingLogged(t *testing.T) {
	queries := stream.NewBufferedStream[types.DNSQuery](10)
	handler := NewQueryLogHandler(dnssvc.HandlerFunc(func(_ context.Context, msg *dns.Msg) (*dns.Msg, error) {
		resp := &dns.Msg{}
		resp.SetReply(msg)
		return resp, nil
	}), queries)

	_, err := handler.Handle(t.Context(), queryFor("example.com"))
	require.NoError(t, err)
	assert.Empty(t, queries.Query(0, 10, nil).Items)
}
