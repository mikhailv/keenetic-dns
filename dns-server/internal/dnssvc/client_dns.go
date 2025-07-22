package dnssvc

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

var _ Resolver = (*dnsClient)(nil)

type dnsClient struct {
	name    string
	address string
	client  dns.Client
}

func NewDNSClient(name string, net string, address string, timeout time.Duration) Resolver {
	return &dnsClient{
		name:    name,
		address: address,
		client: dns.Client{
			Net:     net,
			Timeout: timeout,
		},
	}
}

func (s *dnsClient) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackNamedDuration(s.client.Net+"_client.resolve", s.name)()
	resp, _, err := s.client.ExchangeContext(ctx, msg, s.address)
	return resp, err
}
