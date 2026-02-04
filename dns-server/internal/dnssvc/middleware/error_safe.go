package middleware

import (
	"context"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

var _ dnssvc.Middleware = ErrorSafeResponseMiddleware

func ErrorSafeResponseMiddleware(handler dnssvc.Handler) dnssvc.Handler {
	return dnssvc.HandlerFunc(func(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
		resp, err := handler.Handle(ctx, msg)
		if resp == nil {
			resp = dnssvc.RefusedResponse(msg)
		}
		return resp, err
	})
}
