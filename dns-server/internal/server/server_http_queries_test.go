package server

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
