package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
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

func (s *DoHServer) Start(ctx context.Context) {
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
	mux.HandleFunc("POST /", s.handleRequest)
	return mux
}

func (s *DoHServer) handleRequest(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Type") != dnsMessageMediaType || req.Header.Get("Accept") != dnsMessageMediaType {
		s.errorResponse(w, http.StatusBadRequest, "")
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		s.internalErrorResponse(w, "failed to read body", slog.Any("err", err))
		return
	}

	var dnsReq dns.Msg
	if err = dnsReq.Unpack(body); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "failed to unpack DNS request", slog.Any("err", err))
		return
	}

	dnsResp, dsnRespBytes, err := s.dohService.Send(req.Context(), dnsReq)
	if err != nil {
		s.internalErrorResponse(w, "failed to send DNS request", slog.Any("err", err))
		return
	}

	if dnsResp.Question[0].Qtype == dns.TypeA {
		domain := strings.TrimRight(dnsResp.Question[0].Name, ".")
		if iface, ok := s.config.Routing.LookupHost(domain); ok {
			for _, it := range dnsResp.Answer {
				if a, ok := it.(*dns.A); ok {
					rec := NewDNSRecord(domain, NewIPv4(a.A), time.Duration(a.Hdr.Ttl)*time.Second)
					s.ipRoutes.AddRoute(req.Context(), rec, iface)
				}
			}
		}
		fmt.Printf("\t%s\t%d ip\n", domain, len(dnsResp.Answer))
	}

	w.Header().Set("Content-Type", dnsMessageMediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(dsnRespBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dsnRespBytes)
}

func (s *DoHServer) internalErrorResponse(w http.ResponseWriter, msg string, args ...any) {
	s.errorResponse(w, http.StatusInternalServerError, msg, args...)
}

func (s *DoHServer) errorResponse(w http.ResponseWriter, statusCode int, msg string, args ...any) {
	if msg != "" || len(args) > 0 {
		s.logger.Error(msg, args...)
	}
	w.WriteHeader(statusCode)
}
