package resolvers

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
)

func TestDotEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		address    string
		serverName string
	}{
		{
			name:       "hostname without port",
			endpoint:   "tls://dns.google",
			address:    "dns.google:853",
			serverName: "dns.google",
		},
		{
			name:       "hostname with port",
			endpoint:   "tls://dns.quad9.net:8853",
			address:    "dns.quad9.net:8853",
			serverName: "dns.quad9.net",
		},
		{
			name:       "address with explicit sni",
			endpoint:   "tls://1.1.1.1?sni=cloudflare-dns.com",
			address:    "1.1.1.1:853",
			serverName: "cloudflare-dns.com",
		},
		{
			name:       "address with port and sni",
			endpoint:   "tls://9.9.9.9:853?sni=dns.quad9.net",
			address:    "9.9.9.9:853",
			serverName: "dns.quad9.net",
		},
		{
			name:       "ipv6 address",
			endpoint:   "tls://[2606:4700:4700::1111]?sni=cloudflare-dns.com",
			address:    "[2606:4700:4700::1111]:853",
			serverName: "cloudflare-dns.com",
		},
		{
			name:       "address without sni verifies against the address itself",
			endpoint:   "tls://1.1.1.1",
			address:    "1.1.1.1:853",
			serverName: "1.1.1.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.endpoint)
			require.NoError(t, err)

			address, serverName := dotEndpoint((*config.URL)(u))
			assert.Equal(t, tt.address, address)
			assert.Equal(t, tt.serverName, serverName)
		})
	}
}
