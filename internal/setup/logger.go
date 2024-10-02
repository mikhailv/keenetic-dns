package setup

import (
	"log/slog"
	"os"

	"github.com/mikhailv/keenetic-dns/internal/log"
)

func Logger(debug bool, wrapHandler func(slog.Handler) slog.Handler) *slog.Logger {
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	var handler slog.Handler
	handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	handler = log.NewPrefixHandler(handler)
	if wrapHandler != nil {
		handler = wrapHandler(handler)
	}
	return slog.New(handler)
}
