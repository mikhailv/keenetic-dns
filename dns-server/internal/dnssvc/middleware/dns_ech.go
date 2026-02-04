package middleware

import (
	"context"
	"slices"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

var _ dnssvc.Middleware = DropECHMiddleware

func DropECHMiddleware(handler dnssvc.Handler) dnssvc.Handler {
	return dnssvc.HandlerFunc(func(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
		resp, err := handler.Handle(ctx, req)
		if err == nil {
			for _, rr := range resp.Answer {
				if https, ok := rr.(*dns.HTTPS); ok {
					// delete ECH value from HTTPS record
					https.Value = slices.DeleteFunc(https.Value, func(v dns.SVCBKeyValue) bool {
						_, ok := v.(*dns.SVCBECHConfig)
						return ok
					})
				}
			}
		}
		return resp, err
	})
}
