package srv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type HTTP struct {
	http.Server
	logger *slog.Logger
}

func NewHTTPServer(addr string, logger *slog.Logger, handler http.Handler) HTTP {
	return HTTP{
		Server: http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
			Handler:           handler,
		},
		logger: logger,
	}
}

func (s *HTTP) Serve(ctx context.Context) error {
	context.AfterFunc(ctx, func() {
		s.logger.Info("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("failed to shutdown server", "err", err)
		}
	})

	s.logger.Info("server starting...", "addr", s.Server.Addr)
	if err := s.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}
