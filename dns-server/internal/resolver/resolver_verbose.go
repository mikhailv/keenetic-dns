package resolver

import (
	"context"
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

func NewVerboseDNSResolver(resolver DNSResolver) DNSResolver {
	return verboseDNSResolver{resolver}
}

var _ DNSResolver = verboseDNSResolver{}

type verboseDNSResolver struct {
	resolver DNSResolver
}

func (s verboseDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	fmt.Println(">> DNS:\n" + s.indent(msg.String()))
	resp, err := s.resolver.Resolve(ctx, msg)
	if err != nil {
		fmt.Println("<< DNS error: " + err.Error())
	} else {
		fmt.Println("<< DNS:\n" + s.indent(resp.String()))
	}
	return resp, err
}

func (verboseDNSResolver) indent(text string) string {
	return "\t" + strings.Join(strings.Split(text, "\n"), "\n\t")
}
