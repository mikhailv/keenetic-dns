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
	dnssvc.SetQueryInfo(ctx, dnssvc.QueryInfo{
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

	handler := NewQueryLogHandler(NewSingleFlightHandler(upstream), queries)

	var wg sync.WaitGroup
	for i := range clients {
		wg.Go(func() {
			ctx := ctxutil.WithNewDNSQueryID(
				ctxutil.WithDNSQueryClientIP(t.Context(), types.MustParseIPv4("192.168.1."+string(rune('1'+i)))))
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
	ids := map[types.QueryID]bool{}
	var leader types.QueryID
	var coalesced []types.DNSQuery
	for _, q := range res.Items {
		assert.Equal(t, "example.com.", q.Domain)
		seen[q.ClientIP] = true
		assert.NotZero(t, q.ID, "every query is identified")
		ids[q.ID] = true
		if q.ReusedFrom == 0 {
			leader = q.ID
		} else {
			coalesced = append(coalesced, q)
		}
	}
	assert.Len(t, seen, clients, "each entry must carry the address of the client that asked")
	assert.Len(t, ids, clients, "ids are not reused between queries")

	require.Len(t, coalesced, clients-1, "one client made the exchange, the rest were answered from it")
	require.NotZero(t, leader)
	for _, q := range coalesced {
		assert.Equal(t, leader, q.ReusedFrom, "each names the exchange it was answered from")
	}
}

func TestQueryLog_ReportsResolvedQuery(t *testing.T) {
	upstream := &observingHandler{domain: "example.com."}
	queries := stream.NewBufferedStream[types.DNSQuery](10)
	handler := NewQueryLogHandler(upstream, queries)

	client := types.MustParseIPv4("192.168.1.10")
	ctx := ctxutil.WithNewDNSQueryID(ctxutil.WithDNSQueryClientIP(t.Context(), client))
	_, err := handler.Handle(ctx, queryFor("example.com"))
	require.NoError(t, err)

	res := queries.Query(0, 10, nil)
	require.Len(t, res.Items, 1)
	assert.Equal(t, client, res.Items[0].ClientIP)
	assert.Equal(t, "example.com.", res.Items[0].Domain)
	assert.NotZero(t, res.Items[0].ID)
	assert.Zero(t, res.Items[0].ReusedFrom, "a query that made its own exchange names no other")
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

func TestSingleFlight_FollowersKeepLeaderRcode(t *testing.T) {
	release := make(chan struct{})
	upstream := dnssvc.HandlerFunc(func(_ context.Context, msg *dns.Msg) (*dns.Msg, error) {
		<-release
		resp := &dns.Msg{}
		resp.SetRcode(msg, dns.RcodeNameError)
		return resp, nil
	})
	handler := NewSingleFlightHandler(upstream)

	var mu sync.Mutex
	var codes []int
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			resp, err := handler.Handle(t.Context(), queryFor("absent.example.com"))
			require.NoError(t, err)
			mu.Lock()
			codes = append(codes, resp.Rcode)
			mu.Unlock()
		})
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	require.Len(t, codes, 4)
	for _, c := range codes {
		assert.Equal(t, dns.RcodeNameError, c, "a follower must not be told the name exists")
	}
}
