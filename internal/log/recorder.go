package log

import (
	"context"
	"log/slog"

	"github.com/mikhailv/keenetic-dns/internal/stream"
)

var _ slog.Handler = Recorder{}

type Recorder struct {
	handler slog.Handler
	stream  *stream.Buffered[Entry]
}

func NewRecorder(handler slog.Handler, bufferSize int) Recorder {
	return Recorder{
		handler: handler,
		stream:  stream.NewBufferedStream[Entry](bufferSize),
	}
}

func (s Recorder) Stream() *stream.Buffered[Entry] {
	return s.stream
}

func (s Recorder) Enabled(ctx context.Context, level slog.Level) bool {
	return s.handler.Enabled(ctx, level)
}

func (s Recorder) Handle(ctx context.Context, record slog.Record) error {
	s.stream.Append(NewEntry(record))
	return s.handler.Handle(ctx, record)
}

func (s Recorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	return Recorder{s.handler.WithAttrs(attrs), s.stream}
}

func (s Recorder) WithGroup(name string) slog.Handler {
	return Recorder{s.handler.WithGroup(name), s.stream}
}
