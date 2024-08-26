package internal

import (
	"context"
	"log/slog"
	"time"

	"github.com/miekg/dns"
)

type DNSServer struct {
	logger   *slog.Logger
	config   *Config
	resolver DNSResolver
	server   dns.Server
}

func NewDNSServer(addr string, logger *slog.Logger, config *Config, resolver DNSResolver) *DNSServer {
	return &DNSServer{
		logger:   logger,
		config:   config,
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
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.ShutdownContext(shutdownCtx); err != nil {
			s.logger.Error("dns: failed to shutdown server", slog.Any("err", err))
		}
	}()
	s.server.Handler = s.createHandler(ctx)
	s.logger.Info("dns: server starting...", slog.String("addr", s.server.Addr))
	if err := s.server.ListenAndServe(); err != nil {
		s.logger.Error("dns: failed to start server", slog.Any("err", err))
	}
}

func (s *DNSServer) createHandler(ctx context.Context) dns.Handler {
	return dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		defer TrackDuration("dns.handle")()
		resp, err := s.resolver.Resolve(ctx, req)
		if err != nil {
			s.logger.Error("dns: failed to handle request", slog.Any("err", err))
			resp = &dns.Msg{}
			resp.SetRcode(req, dns.RcodeServerFailure)
			TrackStatus("dns.handle", "failed")
		} else {
			TrackStatus("dns.handle", "success")
		}
		_ = w.WriteMsg(resp)
	})
}
