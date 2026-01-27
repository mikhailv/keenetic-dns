package setup

import (
	"log/slog"
	"os"
)

func Logger(debug bool, wrapHandler func(slog.Handler) slog.Handler) *slog.Logger {
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	if wrapHandler != nil {
		handler = wrapHandler(handler)
	}
	return slog.New(handler)
}
