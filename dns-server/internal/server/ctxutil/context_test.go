package ctxutil

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
			assert.Equal(t, tt.want, parseClientIP(tt.remoteAddr).String())
		})
	}
}

func TestParseClientIPUnsupported(t *testing.T) {
	for _, addr := range []string{"[::1]:5353", "2001:db8::1", "", "not-an-address", "/tmp/dns.sock"} {
		assert.Equal(t, types.IPv4{}, parseClientIP(addr), addr)
	}
}

func TestWithDNSQueryClientAddrString(t *testing.T) {
	ctx := WithDNSQueryClientAddrString(t.Context(), "192.168.1.42:53124")

	ip, ok := GetDNSQueryClientIP(ctx)
	assert.True(t, ok)
	assert.Equal(t, "192.168.1.42", ip.String())
}

func TestParseClientIPDropsPort(t *testing.T) {
	first := parseClientIP("192.168.1.42:1000")
	second := parseClientIP("192.168.1.42:2000")
	assert.Equal(t, first, second)
}
