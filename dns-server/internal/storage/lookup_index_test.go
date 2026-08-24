package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func lookup(domain string, ips ...string) *types.DomainLookup {
	dl := &types.DomainLookup{
		Domain: domain,
		Time:   types.TimestampFromTime(time.Now()),
	}
	for _, ip := range ips {
		dl.IPs = append(dl.IPs, types.DomainIP{IP: types.MustParseIPv4(ip), TTL: 3600})
	}
	return dl
}

func TestLookupIndex_AddAndLookupByIP(t *testing.T) {
	idx := NewLookupIndex(0)
	idx.Add(lookup("example.com.", "1.2.3.4"))

	found := idx.LookupByIP(types.MustParseIPv4("1.2.3.4"))
	require.Len(t, found, 1)
	assert.Equal(t, "example.com.", found[0].Domain)
	assert.Empty(t, idx.LookupByIP(types.MustParseIPv4("5.6.7.8")))
}

func TestLookupIndex_SameDomainDifferentCaseIsOneRecord(t *testing.T) {
	idx := NewLookupIndex(0)
	idx.Add(lookup("example.com.", "1.2.3.4"))
	idx.Add(lookup("Example.COM.", "1.2.3.4"))

	assert.Equal(t, 1, idx.Size(), "case alone must not create a second record")
	assert.Len(t, idx.LookupByIP(types.MustParseIPv4("1.2.3.4")), 1)
}

func TestLookupIndex_DifferentAddressSetsAreSeparate(t *testing.T) {
	idx := NewLookupIndex(0)
	idx.Add(lookup("example.com.", "1.2.3.4"))
	idx.Add(lookup("example.com.", "5.6.7.8"))

	assert.Equal(t, 2, idx.Size(), "an answer with other addresses is its own record")
}

func TestLookupIndex_RemoveMatchesRegardlessOfCase(t *testing.T) {
	idx := NewLookupIndex(0)
	idx.Add(lookup("example.com.", "1.2.3.4"))

	idx.Remove(lookup("EXAMPLE.com.", "1.2.3.4"))

	assert.Equal(t, 0, idx.Size())
	assert.Empty(t, idx.LookupByIP(types.MustParseIPv4("1.2.3.4")))
}
