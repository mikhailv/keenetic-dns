package setup

import (
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/stream"
)

func Logger(debug bool, bufferSize int, file LogFile) (logger *slog.Logger, flush func()) {
	logger, _, flush = setupLogger(debug, bufferSize, 0, file)
	return logger, flush
}

func LoggerStream(debug bool, bufferSize, streamSize int, file LogFile) (logger *slog.Logger, stream *stream.Buffered[log.Entry], flush func()) {
	return setupLogger(debug, bufferSize, streamSize, file)
}

func LogPanic(logger *slog.Logger) {
	if r := recover(); r != nil {
		logger.Error("Panic", "err", r)
	}
}

func setupLogger(debug bool, bufferSize, streamSize int, file LogFile) (logger *slog.Logger, stream *stream.Buffered[log.Entry], flush func()) {
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	var out io.Writer = os.Stdout
	if file.Path != "" {
		out = &lumberjack.Logger{
			Filename:   file.Path,
			MaxSize:    file.MaxSizeMB,
			MaxBackups: file.MaxBackups,
			Compress:   true,
		}
	}
	var handler slog.Handler = slog.NewTextHandler(out, &slog.HandlerOptions{Level: logLevel})
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
