package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
)

type DNSServer struct {
	logger   *slog.Logger
	resolver dnssvc.Resolver
	servers  []*dns.Server
}

func NewDNSServer(addr string, logger *slog.Logger, resolver dnssvc.Resolver) *DNSServer {
	newServer := func(net string) *dns.Server {
		return &dns.Server{
			Addr:         addr,
			Net:          net,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
	}
	return &DNSServer{
		logger:   logger,
		resolver: resolver,
		servers: []*dns.Server{
			newServer("udp"),
			newServer("tcp"),
		},
	}
}

func (s *DNSServer) Serve(ctx context.Context) error {
	handler := s.createHandler(ctx)

	// Cancel on parent shutdown or on the first listener error, so a failure in
	// one transport tears down the others instead of hanging.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	context.AfterFunc(ctx, func() {
		for _, srv := range s.servers {
			s.logger.Info("shutting down server...", "net", srv.Net)
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := srv.ShutdownContext(shutdownCtx); err != nil {
				s.logger.Error("failed to shutdown server", "err", err, "net", srv.Net)
			}
			shutdownCancel()
		}
	})

	errCh := make(chan error, len(s.servers))
	for _, srv := range s.servers {
		srv.Handler = handler
		go func(srv *dns.Server) {
			s.logger.Info("server starting...", "addr", srv.Addr, "net", srv.Net)
			if err := srv.ListenAndServe(); err != nil {
				errCh <- fmt.Errorf("failed to start %s server: %w", srv.Net, err)
				return
			}
			errCh <- nil
		}(srv)
	}

	var firstErr error
	for range s.servers {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
			cancel()
		}
	}

	return firstErr
}

func (s *DNSServer) createHandler(ctx context.Context) dns.Handler {
	return dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		defer metrics.TrackDuration("dns.handle")()
		resp, err := s.resolver.Resolve(ctxutil.WithDNSQueryClientAddrString(ctx, w.RemoteAddr().String()), req)
		if err != nil {
			s.logger.Error("failed to handle request", "err", err)
			metrics.TrackStatus("dns.handle", "failed")
			_ = w.WriteMsg(dnssvc.RefusedResponse(req))
		} else {
			metrics.TrackStatus("dns.handle", "success")
			_ = w.WriteMsg(resp)
		}
	})
}
