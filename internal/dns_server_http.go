package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	dnsMessageMediaType = "application/dns-message"
)

type HTTPServer struct {
	logger   *slog.Logger
	config   *Config
	resolver DNSResolver
	server   http.Server
}

func NewHTTPServer(addr string, logger *slog.Logger, config *Config, resolver DNSResolver) *HTTPServer {
	return &HTTPServer{
		logger:   logger,
		config:   config,
		resolver: resolver,
		server: http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

func (s *HTTPServer) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("http: failed to shutdown server", slog.Any("err", err))
		}
	}()
	s.server.Handler = s.createHandler()
	s.logger.Info("http: server starting...", slog.String("addr", s.server.Addr))
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("http: failed to start server", slog.Any("err", err))
	}
}

func (s *HTTPServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.Handle("POST /dns-query", s.wrapHandler(s.handleDNSQuery))
	return mux
}

func (s *HTTPServer) wrapHandler(handler func(w http.ResponseWriter, req *http.Request) (statusCode int, err error)) http.Handler {
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

func (s *HTTPServer) handleDNSQuery(w http.ResponseWriter, req *http.Request) (statusCode int, err error) {
	if req.Header.Get("Content-Type") != dnsMessageMediaType || req.Header.Get("Accept") != dnsMessageMediaType {
		return http.StatusBadRequest, errors.New("http: unexpected request format")
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("http: failed to read body: %w", err)
	}

	var dnsReq dns.Msg
	if err = dnsReq.Unpack(body); err != nil {
		return http.StatusBadRequest, fmt.Errorf("http: failed to unpack DNS request: %w", err)
	}

	dnsResp, err := s.resolver.Resolve(req.Context(), &dnsReq)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("http: failed to send DNS request: %w", err)
	}

	dnsRespBytes, err := dnsResp.Pack()
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("http: failed to pack DNS response: %w", err)
	}

	w.Header().Set("Content-Type", dnsMessageMediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(dnsRespBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dnsRespBytes)

	return http.StatusOK, nil
}
