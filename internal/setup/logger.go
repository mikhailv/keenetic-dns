package setup

import (
	"log/slog"
	"os"

	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/stream"
)

func Logger(debug bool, bufferSize int) (logger *slog.Logger, flush func()) {
	logger, _, flush = setupLogger(debug, bufferSize, 0)
	return logger, flush
}

func LoggerStream(debug bool, bufferSize, streamSize int) (logger *slog.Logger, stream *stream.Buffered[log.Entry], flush func()) {
	return setupLogger(debug, bufferSize, streamSize)
}

func setupLogger(debug bool, bufferSize, streamSize int) (logger *slog.Logger, stream *stream.Buffered[log.Entry], flush func()) {
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	if bufferSize > 0 {
		buffered := log.NewBufferedHandler(handler, 300)
		flush = buffered.Flush
		handler = buffered
	}
	if streamSize > 0 {
		recorder := log.NewRecorder(handler, streamSize)
		stream = recorder.Stream()
		handler = recorder
	}
	handler = log.NewPrefixHandler(handler)
	return slog.New(handler), stream, flush
}
