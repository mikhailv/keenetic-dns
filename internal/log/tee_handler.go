package log

import (
	"context"
	"errors"
	"log/slog"
)

var _ slog.Handler = TeeHandler{}

type TeeHandler []slog.Handler

func (s TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range s {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (s TeeHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range s {
		if err := handler.Handle(ctx, record); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	res := make(TeeHandler, len(s))
	for i, handler := range s {
		res[i] = handler.WithAttrs(attrs)
	}
	return res
}

func (s TeeHandler) WithGroup(name string) slog.Handler {
	res := make(TeeHandler, len(s))
	for i, handler := range s {
		res[i] = handler.WithGroup(name)
	}
	return res
}
