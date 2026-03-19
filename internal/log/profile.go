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
		logger.Info(fmt.Sprintf("%s completed in %v", msg, time.Since(st).Truncate(time.Microsecond)), args...)
	}
}
