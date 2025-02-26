package resolver

import (
	"context"

	"github.com/miekg/dns"
)

type DNSResolver interface {
	Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error)
}
