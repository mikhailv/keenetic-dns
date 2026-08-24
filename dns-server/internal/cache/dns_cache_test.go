package cache

import (
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	c := NewMemoryDNSCache()
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	c.Put(t.Context(), answer("example.com."))

	resp := c.Get(t.Context(), question("example.com."))
	require.NotNil(t, resp)
	assert.Len(t, resp.Answer, 1)
}

func TestMemDNSCache_IgnoresQuestionCase(t *testing.T) {
	c := NewMemoryDNSCache()
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	c.Put(t.Context(), answer("Example.COM."))

	assert.NotNil(t, c.Get(t.Context(), question("example.com.")), "a lowercase query hits an entry stored with capitals")
	assert.NotNil(t, c.Get(t.Context(), question("EXAMPLE.com.")), "and the other way round")
}

func TestMemDNSCache_MissOnOtherQuestion(t *testing.T) {
	c := NewMemoryDNSCache()
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	c.Put(t.Context(), answer("example.com."))

	assert.Nil(t, c.Get(t.Context(), question("other.example.com.")))
	assert.Nil(t, c.Get(t.Context(), dns.Question{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}))
}
