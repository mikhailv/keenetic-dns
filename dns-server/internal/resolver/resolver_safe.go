package resolver

import (
	"context"

	"github.com/miekg/dns"
)

func NewErrorSafeDNSResolver(resolver DNSResolver) DNSResolver {
	return errorSafeDNSResolver{resolver}
}

type errorSafeDNSResolver struct {
	resolver DNSResolver
}

func (s errorSafeDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resp, err := s.resolver.Resolve(ctx, msg)
	if resp == nil {
		resp = RefusedResponse(msg)
	}
	return resp, err
}
