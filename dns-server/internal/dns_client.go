package internal

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

var _ DNSResolver = (*dnsClient)(nil)

type dnsClient struct {
	address string
	client  dns.Client
}

func NewDNSClient(address string, timeout time.Duration) DNSResolver {
	return &dnsClient{
		address: address,
		client: dns.Client{
			Net:     "udp",
			Timeout: timeout,
		},
	}
}

func (c *dnsClient) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackDuration("dns_client.resolve")()
	resp, _, err := c.client.ExchangeContext(ctx, msg, c.address)
	return resp, err
}
