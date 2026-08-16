package middleware

import (
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
)

var _ dnssvc.Middleware = SingleInflightMiddleware

func SingleInflightMiddleware(handler dnssvc.Handler) dnssvc.Handler {
	return handlers.NewSingleInflightHandler(handler)
}
