package log

import (
	"fmt"
	"log/slog"
	"time"
)

func Profile(logger *slog.Logger, msg string, args ...any) func() {
	st := time.Now()
	logger.Info(msg, args...)
	return func() {
		logger.Debug(fmt.Sprintf("%s completed in %fs", msg, time.Since(st).Seconds()), args...)
	}
}
