package middleware

import (
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
)

func NewTTLOverrideMiddleware(ttl time.Duration) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		if ttl <= 0 {
			return handler
		}
		return handlers.NewTTLOverrideHandler(handler, ttl)
	}
}
