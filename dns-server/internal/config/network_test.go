package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func TestLoadConfig_Network(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
network:
  local_networks: [192.168.2.0/24, 10.10.11.0/24]
  wan_ips: [77.40.49.132]
  tunnel_ips: [5.249.145.107]
`), 0o600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, Network{
		LocalNetworks: []types.IPv4{types.MustParseIPv4("192.168.2.0/24"), types.MustParseIPv4("10.10.11.0/24")},
		WANIPs:        []types.IPv4{types.MustParseIPv4("77.40.49.132")},
		TunnelIPs:     []types.IPv4{types.MustParseIPv4("5.249.145.107")},
	}, cfg.Network)
}
