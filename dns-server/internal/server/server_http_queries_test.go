package server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func directQuery(domain string) types.DNSQuery {
	return types.DNSQuery{Domain: domain, Lookup: &types.DomainLookup{Domain: domain}}
}

func routedQuery(domain string) types.DNSQuery {
	q := directQuery(domain)
	q.IPRoutings.AddRoute("nwg0", "test", types.MustParseIPv4("1.2.3.4"))
	return q
}

func excludedQuery(domain string) types.DNSQuery {
	q := directQuery(domain)
	q.IPRoutings.AddExcluded("test", types.MustParseIPv4("1.2.3.4"))
	return q
}

func blockedQuery(domain string) types.DNSQuery {
	return types.DNSQuery{Domain: domain, Blocked: &types.BlockInfo{List: "test", Domain: domain}}
}

func reusedQuery(domain string) types.DNSQuery {
	q := directQuery(domain)
	q.ReusedFrom = 42
	return q
}

func TestFilterQueries_Status(t *testing.T) {
	all := map[string]types.DNSQuery{
		"direct":   directQuery("plain.example.com"),
		"routed":   routedQuery("routed.example.com"),
		"excluded": excludedQuery("excluded.example.com"),
		"blocked":  blockedQuery("ads.example.com"),
		"reused":   reusedQuery("reused.example.com"),
	}

	tests := []struct {
		status string
		want   []string
	}{
		{status: "blocked", want: []string{"blocked"}},
		{status: "routed", want: []string{"routed"}},
		{status: "excluded", want: []string{"excluded"}},
		{status: "direct", want: []string{"direct", "reused"}},
		{status: "blocked,routed", want: []string{"blocked", "routed"}},
		{status: "reused", want: []string{"reused"}},
		{status: "reused,blocked", want: []string{"reused", "blocked"}},
		{status: " routed , blocked ", want: []string{"routed", "blocked"}},
		{status: "nonsense", want: []string{"direct", "routed", "excluded", "blocked", "reused"}},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			var s HTTPServer
			filter := s.filterQueries(nil, url.Values{"status": {tt.status}})

			var got []string
			for name, query := range all {
				if filter == nil || filter(query) {
					got = append(got, name)
				}
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestFilterQueries_StatusCombinesWithSearch(t *testing.T) {
	var s HTTPServer
	filter := s.filterQueries(nil, url.Values{"status": {"routed"}, "search": {"routed."}})
	require.NotNil(t, filter)

	assert.True(t, filter(routedQuery("routed.example.com")))
	assert.False(t, filter(routedQuery("other.example.com")), "the search still applies")
	assert.False(t, filter(blockedQuery("routed.example.com")), "the status still applies")
}

func TestFilterQueries_NoParamsNoFilter(t *testing.T) {
	var s HTTPServer
	assert.Nil(t, s.filterQueries(nil, url.Values{}))
	assert.Nil(t, s.filterQueries(nil, url.Values{"status": {""}}), "an empty selection filters nothing")
}

func clientQuery(clientIP, domain string) types.DNSQuery {
	q := directQuery(domain)
	q.ClientIP = types.MustParseIPv4(clientIP)
	return q
}

func TestFilterQueries_Client(t *testing.T) {
	var s HTTPServer
	filter := s.filterQueries(nil, url.Values{"client": {"192.168.1.10"}})
	require.NotNil(t, filter)

	assert.True(t, filter(clientQuery("192.168.1.10", "example.com")))
	assert.False(t, filter(clientQuery("192.168.1.11", "example.com")))
}

func TestFilterQueries_ClientInvalidIgnored(t *testing.T) {
	var s HTTPServer
	assert.Nil(t, s.filterQueries(nil, url.Values{"client": {"not-an-ip"}}))
	assert.Nil(t, s.filterQueries(nil, url.Values{"client": {""}}))
}

func TestFilterQueries_Search(t *testing.T) {
	query := clientQuery("192.168.1.10", "video.example.com")
	query.ID = types.NewQueryID(time.Now(), 1)
	query.Lookup.CNames = []types.DomainEntry[string]{{Name: "cdn.Example.NET", TTL: 60}}
	query.Lookup.IPs = []types.DomainIP{{IP: types.MustParseIPv4("93.184.216.34"), TTL: 60}}
	blocked := blockedQuery("ads.example.com")
	blocked.Blocked.List = "adlist"

	tests := []struct {
		search string
		val    types.DNSQuery
		want   bool
	}{
		{search: "video.exa", val: query, want: true},
		{search: "VIDEO", val: query, want: true},
		{search: "cdn.example.net", val: query, want: true},
		{search: "93.184", val: query, want: true},
		{search: "192.168.1.10", val: query, want: true},
		{search: query.ID.String(), val: query, want: true},
		{search: "nothing", val: query, want: false},
		{search: "adlist", val: blocked, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.search, func(t *testing.T) {
			var s HTTPServer
			filter := s.filterQueries(nil, url.Values{"search": {tt.search}})
			require.NotNil(t, filter)
			assert.Equal(t, tt.want, filter(tt.val))
		})
	}
}

func TestFilterQueries_SearchMatchesRoutedIP(t *testing.T) {
	var s HTTPServer
	filter := s.filterQueries(nil, url.Values{"search": {"1.2.3.4"}})
	require.NotNil(t, filter)

	assert.True(t, filter(routedQuery("routed.example.com")))
	assert.False(t, filter(directQuery("plain.example.com")))
}

func TestFilterQueries_ClientCombinesWithSearch(t *testing.T) {
	var s HTTPServer
	filter := s.filterQueries(nil, url.Values{"client": {"192.168.1.10"}, "search": {"video"}})
	require.NotNil(t, filter)

	assert.True(t, filter(clientQuery("192.168.1.10", "video.example.com")))
	assert.False(t, filter(clientQuery("192.168.1.11", "video.example.com")), "the client still applies")
	assert.False(t, filter(clientQuery("192.168.1.10", "other.example.com")), "the search still applies")
}

type hostsClient struct {
	agentclient.NetworkServiceClient
	hosts []agentclient.HostInfo
	err   error
	calls int
	block chan struct{}
}

func (c *hostsClient) ListHosts(context.Context) ([]agentclient.HostInfo, error) {
	if c.block != nil {
		<-c.block
	}
	c.calls++
	return c.hosts, c.err
}

func serverWithHosts(t *testing.T, client *hostsClient) *HTTPServer {
	t.Helper()
	s := &HTTPServer{logger: slog.New(slog.DiscardHandler), networkService: client}
	s.refreshHosts(t.Context())
	return s
}

func TestFilterQueries_SearchMatchesClientName(t *testing.T) {
	s := serverWithHosts(t, &hostsClient{hosts: []agentclient.HostInfo{
		{Ip: new("192.168.1.10"), Name: new("Living Room TV")},
		{Ip: new("192.168.1.11"), Hostname: new("laptop")},
		{Name: new("no-ip-host")},
	}})

	filter := s.filterQueries(nil, url.Values{"search": {"living room"}})
	require.NotNil(t, filter)
	assert.True(t, filter(clientQuery("192.168.1.10", "example.com")))
	assert.False(t, filter(clientQuery("192.168.1.11", "example.com")))

	filter = s.filterQueries(nil, url.Values{"search": {"laptop"}})
	require.NotNil(t, filter)
	assert.True(t, filter(clientQuery("192.168.1.11", "example.com")))
}

func TestFilterQueries_SearchServesCachedHosts(t *testing.T) {
	client := &hostsClient{hosts: []agentclient.HostInfo{{Ip: new("192.168.1.10"), Name: new("tv")}}}
	s := serverWithHosts(t, client)

	for range 3 {
		filter := s.filterQueries(nil, url.Values{"search": {"tv"}})
		require.NotNil(t, filter)
		assert.True(t, filter(clientQuery("192.168.1.10", "example.com")))
	}
	assert.Equal(t, 1, client.calls, "the host list is refreshed on a schedule, not per request")
}

func TestFilterQueries_SearchUsesHostsFromFailedRefresh(t *testing.T) {
	client := &hostsClient{hosts: []agentclient.HostInfo{{Ip: new("192.168.1.10"), Name: new("tv")}}}
	s := serverWithHosts(t, client)

	client.hosts, client.err = nil, errors.New("agent unavailable")
	s.refreshHosts(t.Context())

	filter := s.filterQueries(nil, url.Values{"search": {"tv"}})
	require.NotNil(t, filter)
	assert.True(t, filter(clientQuery("192.168.1.10", "example.com")), "the last known host list is kept")
}

func TestFilterQueries_SearchSurvivesHostListFailure(t *testing.T) {
	s := serverWithHosts(t, &hostsClient{err: errors.New("agent unavailable")})

	filter := s.filterQueries(nil, url.Values{"search": {"example"}})
	require.NotNil(t, filter)
	assert.True(t, filter(clientQuery("192.168.1.10", "example.com")), "the rest of the search still works")
}

func TestHostList_NotLoaded(t *testing.T) {
	var s HTTPServer
	hosts, err := s.hostList()
	assert.Nil(t, hosts)
	require.ErrorIs(t, err, errHostsNotLoaded)
}

func TestHostList_NotLoadedUntilFirstSuccess(t *testing.T) {
	client := &hostsClient{err: errors.New("agent unavailable")}
	s := serverWithHosts(t, client)

	hosts, err := s.hostList()
	assert.Nil(t, hosts)
	require.ErrorIs(t, err, errHostsNotLoaded)

	client.hosts, client.err = []agentclient.HostInfo{{Ip: new("192.168.1.10")}}, nil
	s.refreshHosts(t.Context())

	hosts, err = s.hostList()
	require.NoError(t, err)
	assert.Len(t, hosts, 1)
}

func TestRefreshHosts_StaysQuietWhenContextIsCancelled(t *testing.T) {
	var logs bytes.Buffer
	s := &HTTPServer{
		logger:         slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})),
		networkService: &hostsClient{err: context.Canceled},
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.refreshHosts(ctx)

	assert.Empty(t, logs.String())
}

func TestStartHostRefresh_DoesNotBlockCaller(t *testing.T) {
	release := make(chan struct{})
	s := &HTTPServer{
		logger:         slog.New(slog.DiscardHandler),
		networkService: &hostsClient{block: release},
	}

	ctx, cancel := context.WithCancel(t.Context())
	waiter := s.startHostRefresh(ctx)

	_, err := s.hostList()
	require.ErrorIs(t, err, errHostsNotLoaded, "the first refresh is still running")

	close(release)
	cancel()
	waiter.Wait()

	hosts, err := s.hostList()
	require.NoError(t, err)
	assert.Empty(t, hosts)
}
