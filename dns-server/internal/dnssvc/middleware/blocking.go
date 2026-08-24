package middleware

import (
	"log/slog"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/blocklist"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
)

var _ handlers.Blocklist = (*blocklist.Manager)(nil)

func NewBlockingMiddleware(
	list handlers.Blocklist,
	recorder handlers.StatsRecorder,
	logger *slog.Logger,
) dnssvc.Middleware {
	return func(handler dnssvc.Handler) dnssvc.Handler {
		return handlers.NewBlockingHandler(handler, list, recorder, logger)
	}
}
