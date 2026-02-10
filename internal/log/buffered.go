package log

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

var _ slog.Handler = &BufferedHandler{}

type logRecord struct {
	ctx     context.Context //nolint:containedctx // store context to call slog.Handler later
	handler slog.Handler
	record  slog.Record
}

type bufferedHandlerState struct {
	mu  sync.Mutex
	buf util.RingBuf[logRecord]
}

type BufferedHandler struct {
	handler slog.Handler
	*bufferedHandlerState
}

func NewBufferedHandler(handler slog.Handler, bufferSize int) *BufferedHandler {
	return &BufferedHandler{
		handler: handler,
		bufferedHandlerState: &bufferedHandlerState{
			buf: *util.NewRingBuf[logRecord](bufferSize),
		},
	}
}

func (s *BufferedHandler) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.flush()
}

func (s *BufferedHandler) flush() error {
	if s.buf.Size() == 0 {
		return nil
	}
	var errs []error
	for it := range s.buf.Iterator(0, 1) {
		if err := it.handler.Handle(it.ctx, it.record); err != nil {
			errs = append(errs, err)
		}
	}
	s.buf.Clear()
	return errors.Join(errs...)
}

func (s *BufferedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return s.handler.Enabled(ctx, level)
}

func (s *BufferedHandler) Handle(ctx context.Context, record slog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Add(logRecord{ctx, s.handler, record.Clone()})
	if s.buf.Size() == s.buf.Capacity() {
		return s.flush()
	}
	return nil
}

func (s *BufferedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BufferedHandler{s.handler.WithAttrs(attrs), s.bufferedHandlerState}
}

func (s *BufferedHandler) WithGroup(name string) slog.Handler {
	return &BufferedHandler{s.handler.WithGroup(name), s.bufferedHandlerState}
}
