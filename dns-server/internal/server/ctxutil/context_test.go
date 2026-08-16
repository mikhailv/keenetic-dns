package ctxutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func TestClientIPRoundTrip(t *testing.T) {
	ctx := WithDNSQueryClientIP(t.Context(), types.MustParseIPv4("192.168.1.42"))

	ip, ok := GetDNSQueryClientIP(ctx)
	assert.True(t, ok)
	assert.Equal(t, "192.168.1.42", ip.String())
}

func TestClientIPAbsent(t *testing.T) {
	ip, ok := GetDNSQueryClientIP(t.Context())
	assert.False(t, ok)
	assert.Equal(t, types.IPv4{}, ip)
}

func TestParseClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{name: "udp address with port", remoteAddr: "192.168.1.42:53124", want: "192.168.1.42"},
		{name: "bare address", remoteAddr: "192.168.1.42", want: "192.168.1.42"},
		{name: "loopback", remoteAddr: "127.0.0.1:5353", want: "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, ok := parseClientIP(tt.remoteAddr)
			require.True(t, ok)
			assert.Equal(t, tt.want, ip.String())
		})
	}
}

func TestParseClientIPUnsupported(t *testing.T) {
	for _, addr := range []string{"[::1]:5353", "2001:db8::1", "", "not-an-address", "/tmp/dns.sock"} {
		_, ok := parseClientIP(addr)
		assert.False(t, ok, addr)
	}
}

func TestClientAddrStringLeavesUnsupportedUnset(t *testing.T) {
	ctx := WithDNSQueryClientAddrString(t.Context(), "[fd00::10]:38422")

	_, ok := GetDNSQueryClientIP(ctx)
	assert.False(t, ok, "an address that is not IPv4 must not read as the zero client")

	id, ok := GetDNSQueryID(ctx)
	require.True(t, ok, "the query is still identified")
	assert.NotZero(t, id)
}

func TestWithDNSQueryClientAddrString(t *testing.T) {
	ctx := WithDNSQueryClientAddrString(t.Context(), "192.168.1.42:53124")

	ip, ok := GetDNSQueryClientIP(ctx)
	assert.True(t, ok)
	assert.Equal(t, "192.168.1.42", ip.String())
}

func TestParseClientIPDropsPort(t *testing.T) {
	first, _ := parseClientIP("192.168.1.42:1000")
	second, _ := parseClientIP("192.168.1.42:2000")
	assert.Equal(t, first, second)
}

func TestDNSQueryID(t *testing.T) {
	ctx := WithNewDNSQueryID(t.Context())
	id, ok := GetDNSQueryID(ctx)
	require.True(t, ok)
	assert.NotZero(t, id)

	other, ok := GetDNSQueryID(WithNewDNSQueryID(t.Context()))
	require.True(t, ok)
	assert.NotEqual(t, id, other)
}

func TestDNSQueryIDAbsent(t *testing.T) {
	id, ok := GetDNSQueryID(t.Context())
	assert.False(t, ok)
	assert.Zero(t, id)
}

func TestClientAddrStringAssignsQueryID(t *testing.T) {
	ctx := WithDNSQueryClientAddrString(t.Context(), "192.168.1.10:5353")
	id, ok := GetDNSQueryID(ctx)
	require.True(t, ok)
	assert.NotZero(t, id)
}
