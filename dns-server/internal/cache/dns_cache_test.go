package cache

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func answer(domain string) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(domain, dns.TypeA)
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   []byte{1, 2, 3, 4},
	}}
	return resp
}

func question(domain string) dns.Question {
	return dns.Question{Name: domain, Qtype: dns.TypeA, Qclass: dns.ClassINET}
}

func TestMemDNSCache_RoundTrip(t *testing.T) {
	c := NewMemoryDNSCache(Config{})
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	c.Put(t.Context(), answer("example.com."))

	resp := c.Get(t.Context(), question("example.com."))
	require.NotNil(t, resp)
	assert.Len(t, resp.Answer, 1)
}

func TestMemDNSCache_IgnoresQuestionCase(t *testing.T) {
	c := NewMemoryDNSCache(Config{})
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	c.Put(t.Context(), answer("Example.COM."))

	assert.NotNil(t, c.Get(t.Context(), question("example.com.")), "a lowercase query hits an entry stored with capitals")
	assert.NotNil(t, c.Get(t.Context(), question("EXAMPLE.com.")), "and the other way round")
}

func TestMemDNSCache_MissOnOtherQuestion(t *testing.T) {
	c := NewMemoryDNSCache(Config{})
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	c.Put(t.Context(), answer("example.com."))

	assert.Nil(t, c.Get(t.Context(), question("other.example.com.")))
	assert.Nil(t, c.Get(t.Context(), dns.Question{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}))
}

func newTestCache(t *testing.T, extraTTL time.Duration) *memDNSCache {
	t.Helper()
	c, ok := NewMemoryDNSCache(Config{ExtraTTL: extraTTL}).(*memDNSCache)
	require.True(t, ok)
	t.Cleanup(func() { assert.NoError(t, c.Close()) })
	return c
}

func putExpired(t *testing.T, c *memDNSCache, domain string, expiredAgo time.Duration) {
	t.Helper()
	b, err := answer(domain).Pack()
	require.NoError(t, err)
	now := time.Now()
	c.entries[question(domain)] = dnsCacheEntry{
		Bytes:   b,
		Added:   uint32(now.Add(-expiredAgo - time.Minute).Unix()),
		Expires: uint32(now.Add(-expiredAgo).Unix()),
	}
}

func TestMemDNSCache_KeepsExpiredForExtraTTL(t *testing.T) {
	c := newTestCache(t, time.Hour)
	putExpired(t, c, "recent.example.com.", time.Minute)
	putExpired(t, c, "old.example.com.", 2*time.Hour)

	assert.Nil(t, c.Get(t.Context(), question("recent.example.com.")), "an expired answer is never served")

	c.removeExpired()
	assert.Contains(t, c.entries, question("recent.example.com."), "kept for the extra time")
	assert.NotContains(t, c.entries, question("old.example.com."), "dropped once the extra time has passed")
}

func TestMemDNSCache_SaveLoadKeepsEntriesWithinExtraTTL(t *testing.T) {
	c := newTestCache(t, time.Hour)
	putExpired(t, c, "recent.example.com.", time.Minute)
	putExpired(t, c, "old.example.com.", 2*time.Hour)
	c.Put(t.Context(), answer("fresh.example.com."))

	var buf bytes.Buffer
	n, err := c.Save(&buf)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	loaded := newTestCache(t, time.Hour)
	n, err = loaded.Load(&buf)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Contains(t, loaded.entries, question("recent.example.com."))
	assert.NotNil(t, loaded.Get(t.Context(), question("fresh.example.com.")))
}

func TestMemDNSCache_WithoutExtraTTLDropsExpired(t *testing.T) {
	c := newTestCache(t, 0)
	putExpired(t, c, "recent.example.com.", time.Minute)

	c.removeExpired()
	assert.Empty(t, c.entries)
}

func TestMemDNSCache_LookupIP(t *testing.T) {
	c := newTestCache(t, time.Hour)
	c.Put(t.Context(), answer("WWW.Example.com."))

	assert.Equal(t, []string{"www.example.com"}, c.LookupIP(types.MustParseIPv4("1.2.3.4")))
	assert.Empty(t, c.LookupIP(types.MustParseIPv4("1.2.3.5")))
}

func TestMemDNSCache_LookupIPReturnsAllNamesNewestFirst(t *testing.T) {
	c := newTestCache(t, time.Hour)
	c.Put(t.Context(), answer("a.example.com."))
	c.Put(t.Context(), answer("b.example.com."))
	c.Put(t.Context(), answer("a.example.com."))

	assert.Equal(t, []string{"a.example.com", "b.example.com"}, c.LookupIP(types.MustParseIPv4("1.2.3.4")),
		"a name resolved again moves to the front instead of being repeated")

	for i := range 20 {
		c.Put(t.Context(), answer(fmt.Sprintf("cdn%d.example.com.", i)))
	}
	names := c.LookupIP(types.MustParseIPv4("1.2.3.4"))
	assert.Len(t, names, maxNamesPerIP)
	assert.Equal(t, "cdn19.example.com", names[0])
}

func TestMemDNSCache_LookupIPRespectsExtraTTL(t *testing.T) {
	c := newTestCache(t, time.Hour)
	b, err := answer("recent.example.com.").Pack()
	require.NoError(t, err)
	var msg dns.Msg
	require.NoError(t, msg.Unpack(b))
	now := time.Now()

	c.indexAnswers(&msg, dnsCacheEntry{Expires: uint32(now.Add(-time.Minute).Unix())})
	assert.NotEmpty(t, c.LookupIP(types.MustParseIPv4("1.2.3.4")), "an expired answer still names the IP for the extra time")

	c.indexAnswers(&msg, dnsCacheEntry{Expires: uint32(now.Add(-2 * time.Hour).Unix())})
	assert.Empty(t, c.LookupIP(types.MustParseIPv4("1.2.3.4")), "not after the extra time")
	c.removeExpired()
	assert.Empty(t, c.byIP)
}

func TestMemDNSCache_LoadRebuildsIPIndex(t *testing.T) {
	c := newTestCache(t, time.Hour)
	c.Put(t.Context(), answer("example.com."))
	var buf bytes.Buffer
	_, err := c.Save(&buf)
	require.NoError(t, err)

	loaded := newTestCache(t, time.Hour)
	_, err = loaded.Load(&buf)
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com"}, loaded.LookupIP(types.MustParseIPv4("1.2.3.4")))
}
