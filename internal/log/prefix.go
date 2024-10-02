package log

import (
	"context"
	"log/slog"
)

func NewPrefixHandler(handler slog.Handler) slog.Handler {
	return prefixHandler{handler, ""}
}

func WithPrefix(logger *slog.Logger, prefix string) *slog.Logger {
	return logger.With(slog.String(prefixKey, prefix))
}

const prefixKey = "_prefix_"

var _ slog.Handler = prefixHandler{}

type prefixHandler struct {
	handler slog.Handler
	prefix  string
}

func (s prefixHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return s.handler.Enabled(ctx, level)
}

func (s prefixHandler) Handle(ctx context.Context, record slog.Record) error {
	if s.prefix != "" {
		record.Message = s.prefix + ": " + record.Message
	}
	return s.handler.Handle(ctx, record)
}

func (s prefixHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 1 {
		if attrs[0].Key == prefixKey {
			prefix := attrs[0].Value.String()
			if s.prefix != "" {
				prefix = s.prefix + "." + prefix
			}
			return prefixHandler{s.handler, prefix}
		}
	}
	return s
}

func (s prefixHandler) WithGroup(name string) slog.Handler {
	return prefixHandler{s.handler.WithGroup(name), s.prefix}
}
