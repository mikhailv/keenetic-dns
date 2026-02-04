package middleware

import (
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/routing"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/storage"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
)

func NewIPRoutingMiddleware(dnsStore *storage.DNSStore, ipRoutes *routing.IPRouteController, stream stream.Stream[types.DNSQuery]) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		return handlers.NewIPRoutingHandler(handler, dnsStore, ipRoutes, stream)
	}
}
