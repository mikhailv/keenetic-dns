package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	dnsMessageMediaType = "application/dns-message"
)

type DoHServer struct {
	server     http.Server
	logger     *slog.Logger
	config     *Config
	dohService *DoHClient
	ipRoutes   *IPRouteController
}

func NewDoHServer(
	addr string,
	logger *slog.Logger,
	config *Config,
	dohService *DoHClient,
	ipRoutes *IPRouteController,
) *DoHServer {
	s := &DoHServer{
		logger:     logger,
		config:     config,
		dohService: dohService,
		ipRoutes:   ipRoutes,
	}
	s.server = http.Server{
		Addr:              addr,
		Handler:           s.createHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

func (s *DoHServer) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(ctx); err != nil {
			s.logger.Error("failed to shutdown server", slog.Any("err", err))
		}
	}()
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("failed to start server", slog.Any("err", err))
	}
}

func (s *DoHServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.Handle("POST /dns-query", s.wrapHandler(s.handleDNSQuery))
	return mux
}

func (s *DoHServer) wrapHandler(handler func(w http.ResponseWriter, req *http.Request) (statusCode int, err error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path := r.Method, r.URL.Path
		operation := fmt.Sprintf("%s %s", method, path)
		defer TrackDuration(operation)()
		statusCode, err := handler(w, r)
		if err != nil {
			w.WriteHeader(statusCode)
			s.logger.Error(err.Error(), "method", method, "path", path, "statusCode", statusCode)
		}
		TrackStatus(operation, strconv.Itoa(statusCode))
	})
}
