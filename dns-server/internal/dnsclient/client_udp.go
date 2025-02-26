package dnsclient

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/resolver"
)

var _ resolver.DNSResolver = (*udpClient)(nil)

type udpClient struct {
	name    string
	address string
	client  dns.Client
}

func NewUDPClient(name string, address string, timeout time.Duration) resolver.DNSResolver {
	return &udpClient{
		name:    name,
		address: address,
		client: dns.Client{
			Net:     "udp",
			Timeout: timeout,
		},
	}
}

func (s *udpClient) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackNamedDuration("udp_client.resolve", s.name)()
	resp, _, err := s.client.ExchangeContext(ctx, msg, s.address)
	return resp, err
}
