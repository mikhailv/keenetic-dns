package middleware

import (
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
)

func NewRawQueryMiddleware(stream stream.Stream[types.DNSRawQuery]) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		return handlers.NewRawQueryHandler(handler, stream)
	}
}
