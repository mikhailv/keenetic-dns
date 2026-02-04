package middleware

import (
	"github.com/mikhailv/keenetic-dns/dns-server/internal/cache"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
)

func NewCacheMiddleware(cache cache.DNSCache) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		return handlers.NewCachedHandler(handler, cache)
	}
}
