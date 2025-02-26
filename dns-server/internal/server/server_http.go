package server

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/resolver"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/routing"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/stream"
)

const dnsMessageMediaType = "application/dns-message"

type FilterFunc[T any] func(val T) bool

type HTTPServer struct {
	logger         *slog.Logger
	resolver       resolver.DNSResolver
	server         http.Server
	ipRoutes       *routing.IPRouteController
	logStream      *stream.Buffered[log.Entry]
	queryStream    *stream.Buffered[types.DNSQuery]
	rawQueryStream *stream.Buffered[types.DNSRawQuery]
}

func NewHTTPServer(
	addr string,
	logger *slog.Logger,
	resolver resolver.DNSResolver,
	ipRoutes *routing.IPRouteController,
	logStream *stream.Buffered[log.Entry],
	queryStream *stream.Buffered[types.DNSQuery],
	rawQueryStream *stream.Buffered[types.DNSRawQuery],
) *HTTPServer {
	return &HTTPServer{
		logger:   logger,
		resolver: resolver,
		server: http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
		},
		ipRoutes:       ipRoutes,
		logStream:      logStream,
		queryStream:    queryStream,
		rawQueryStream: rawQueryStream,
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
	wsLogger := log.WithPrefix(s.logger, "ws")

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.Handle("POST /dns-query", s.wrapHandler(s.handleDNSQuery))
	mux.Handle("GET /api/routes", http.HandlerFunc(s.handleRoutes))
	mux.Handle("GET /api/logs", createListHandler(s.logStream, s.filterLogs))
	mux.Handle("GET /api/logs/ws", createStreamHandler(s.logStream, wsLogger, s.filterLogs))
	mux.Handle("GET /api/dns-queries", createListHandler(s.queryStream, s.filterQueries))
	mux.Handle("GET /api/dns-queries/ws", createStreamHandler(s.queryStream, wsLogger, s.filterQueries))
	mux.Handle("GET /api/dns-raw-queries", createListHandler(s.rawQueryStream, s.filterRawQueries))
	mux.Handle("GET /api/dns-raw-queries/ws", createStreamHandler(s.rawQueryStream, wsLogger, s.filterRawQueries))
	mux.Handle("GET /app.js", staticFileHandler("app.js"))
	mux.Handle("GET /", staticFileHandler("index.html"))

	return cors.Default().Handler(mux)
}

func (s *HTTPServer) wrapHandler(handler func(w http.ResponseWriter, req *http.Request) (statusCode int, err error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path := r.Method, r.URL.Path
		operation := fmt.Sprintf("%s %s", method, path)
		defer metrics.TrackDuration(operation)()
		statusCode, err := handler(w, r)
		if err != nil {
			w.WriteHeader(statusCode)
			s.logger.Error(err.Error(), "method", method, "path", path, "statusCode", statusCode)
		}
		metrics.TrackStatus(operation, strconv.Itoa(statusCode))
	})
}
