package middleware

import (
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
)

func NewResolveStatsMiddleware(recorder handlers.ResolveRecorder) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		return handlers.NewResolveStatsHandler(handler, recorder)
	}
}
