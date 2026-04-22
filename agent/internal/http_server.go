package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/klauspost/compress/gzhttp"
	"github.com/klauspost/compress/gzip"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"

	"github.com/mikhailv/keenetic-dns/agent/internal/api"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type HTTPServer struct {
	logger         *slog.Logger
	server         http.Server
	networkService api.StrictServerInterface
}

func NewHTTPServer(addr string, logger *slog.Logger, networkService api.StrictServerInterface) *HTTPServer {
	return &HTTPServer{
		logger: logger,
		server: http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
		},
		networkService: networkService,
	}
}

func (s *HTTPServer) Serve(ctx context.Context) error {
	s.server.Handler = s.createHandler()

	context.AfterFunc(ctx, func() {
		s.logger.Info("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("failed to shutdown server", "err", err)
		}
	})

	s.logger.Info("server starting...", "addr", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

func (s *HTTPServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())

	apiServer := api.NewStrictHandler(s.networkService, nil)

	handler := cors.Default().Handler(api.HandlerFromMux(apiServer, mux))
	gzWrapper := util.UnwrapResult(gzhttp.NewWrapper(gzhttp.CompressionLevel(gzip.BestSpeed)))
	return gzWrapper(handler)
}
