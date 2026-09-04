package dnssvc

import (
	"context"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	name  string
	delay time.Duration
	resp  *dns.Msg
}

func (s fakeProvider) Name() string {
	return s.name
}

func (s fakeProvider) Resolve(ctx context.Context, _ *dns.Msg) (*dns.Msg, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
		return s.resp, nil
	}
}

func (s fakeProvider) Close() error {
	return nil
}

func (s fakeProvider) MatchQuery(*dns.Msg) QueryMatchResult {
	return QueryMatchResult{}
}

func query() *dns.Msg {
	msg := &dns.Msg{}
	msg.SetQuestion("tr.rbxcdn.com.", dns.TypeA)
	return msg
}

func cnameOnlyResponse() *dns.Msg {
	return response(dns.TypeA, cname("tr.rbxcdn.com.", "trns1.rbxcdn.com."))
}

func completeResponse() *dns.Msg {
	return response(dns.TypeA,
		cname("tr.rbxcdn.com.", "trns1.rbxcdn.com."),
		cname("trns1.rbxcdn.com.", "traws.rbxcdn.com."),
		a("traws.rbxcdn.com.", "3.164.230.91"),
	)
}

func TestMultiProviderResolver_PrefersCompleteAnswerOverFasterCNAMEOnly(t *testing.T) {
	resolver := NewMultiProviderResolver([]Provider{
		fakeProvider{name: "fast", resp: cnameOnlyResponse()},
		fakeProvider{name: "slow", delay: 50 * time.Millisecond, resp: completeResponse()},
	})

	resp, err := resolver.Resolve(WithResolverInfo(t.Context()), query())
	require.NoError(t, err)
	assert.Equal(t, completeResponse().Answer, resp.Answer)
}

func TestMultiProviderResolver_FallsBackToCNAMEOnly(t *testing.T) {
	resolver := NewMultiProviderResolver([]Provider{
		fakeProvider{name: "fast", resp: cnameOnlyResponse()},
		fakeProvider{name: "slow", delay: 50 * time.Millisecond, resp: cnameOnlyResponse()},
	})

	resp, err := resolver.Resolve(WithResolverInfo(t.Context()), query())
	require.NoError(t, err)
	assert.Equal(t, cnameOnlyResponse().Answer, resp.Answer)
}
