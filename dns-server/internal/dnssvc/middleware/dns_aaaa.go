package middleware

import (
	"context"
	"slices"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

var _ dnssvc.Middleware = DropAAAAMiddleware

func DropAAAAMiddleware(handler dnssvc.Handler) dnssvc.Handler {
	return dnssvc.HandlerFunc(func(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
		resp, err := handler.Handle(ctx, req)
		if err == nil && resp != nil {
			resp.Answer = slices.DeleteFunc(resp.Answer, func(rr dns.RR) bool {
				return rr.Header().Rrtype == dns.TypeAAAA
			})
		}
		return resp, err
	})
}
