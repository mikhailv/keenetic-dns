package middleware

import (
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
)

var _ dnssvc.Middleware = SingleFlightMiddleware

func SingleFlightMiddleware(handler dnssvc.Handler) dnssvc.Handler {
	return handlers.NewSingleFlightHandler(handler)
}
