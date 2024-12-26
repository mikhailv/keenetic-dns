package internal

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"

	"github.com/mikhailv/keenetic-dns/agent/rpc/v1/agentv1connect"
)

type HTTPServer struct {
	logger         *slog.Logger
	server         http.Server
	networkService agentv1connect.NetworkServiceHandler
}

func NewHTTPServer(addr string, logger *slog.Logger, networkService agentv1connect.NetworkServiceHandler) *HTTPServer {
	return &HTTPServer{
		logger: logger,
		server: http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
		},
		networkService: networkService,
	}
}

func (s *HTTPServer) Serve(ctx context.Context) {
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
		s.logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}
}

func (s *HTTPServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.Handle("/api/", http.StripPrefix("/api", s.createAPIHandler()))
	return cors.Default().Handler(mux)
}

func (s *HTTPServer) createAPIHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(agentv1connect.NewNetworkServiceHandler(s.networkService))
	return mux
}
