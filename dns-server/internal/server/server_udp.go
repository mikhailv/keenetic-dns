package server

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/resolver"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
)

type DNSServer struct {
	logger   *slog.Logger
	resolver resolver.DNSResolver
	server   dns.Server
}

func NewDNSServer(addr string, logger *slog.Logger, resolver resolver.DNSResolver) *DNSServer {
	return &DNSServer{
		logger:   logger,
		resolver: resolver,
		server: dns.Server{
			Addr:         addr,
			Net:          "udp",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}
}

func (s *DNSServer) Serve(ctx context.Context) {
	s.server.Handler = s.createHandler(ctx)

	context.AfterFunc(ctx, func() {
		s.logger.Info("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.ShutdownContext(shutdownCtx); err != nil {
			s.logger.Error("failed to shutdown server", "err", err)
		}
	})

	s.logger.Info("server starting...", "addr", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil {
		s.logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}
}

func (s *DNSServer) createHandler(ctx context.Context) dns.Handler {
	return dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		defer metrics.TrackDuration("dns.handle")()
		resp, err := s.resolver.Resolve(ctxutil.WithDNSQueryRemoteAddr(ctx, w.RemoteAddr().String()), req)
		if err != nil {
			s.logger.Error("failed to handle request", "err", err)
			metrics.TrackStatus("dns.handle", "failed")
			_ = w.WriteMsg(resolver.RefusedResponse(req))
		} else {
			metrics.TrackStatus("dns.handle", "success")
			_ = w.WriteMsg(resp)
		}
	})
}
