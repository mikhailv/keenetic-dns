package internal

import (
	"context"

	"github.com/miekg/dns"
)

var _ DNSResolver = dnsClient{}

type dnsClient struct {
	address string
}

func NewDNSClient(address string) DNSResolver {
	return &dnsClient{address}
}

func (c dnsClient) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	defer TrackDuration("dns_client.resolve")()

	return dns.ExchangeContext(ctx, msg, c.address)
}
