package middleware

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

func NewTTLOverrideMiddleware(ttl time.Duration) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		if ttl <= 0 {
			return handler
		}
		return func(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
			resp, err := handler(ctx, req)
			if err != nil {
				return nil, err
			}
			ttlOverride := uint32(ttl.Seconds())
			if ttlOverride > 0 {
				for _, rr := range resp.Answer {
					rr.Header().Ttl = min(rr.Header().Ttl, ttlOverride)
				}
			}
			return resp, nil
		}
	}
}
