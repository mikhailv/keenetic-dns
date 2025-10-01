package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/brutella/dnssd"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
)

type MDNSServer struct {
	logger    *slog.Logger
	cfg       *config.Dynamic[[]config.MDNSService]
	iface     string
	responder dnssd.Responder

	mu struct {
		sync.Mutex
		handles []dnssd.ServiceHandle
	}
}

func NewMDNSServer(logger *slog.Logger, iface string, cfg *config.Dynamic[[]config.MDNSService]) *MDNSServer {
	return &MDNSServer{
		logger: logger,
		cfg:    cfg,
		iface:  iface,
	}
}

func (s *MDNSServer) Serve(ctx context.Context) error {
	var err error
	if s.responder, err = dnssd.NewResponder(); err != nil {
		return fmt.Errorf("mdns: failed to create responder: %w", err)
	}

	s.logger.Info("server starting...", "iface", s.iface)

	s.refresh()
	s.cfg.Listen(s.refresh)

	if err = s.responder.Respond(ctx); err != nil {
		return fmt.Errorf("mdns: failed to start server: %w", err)
	}
	return nil
}

func (s *MDNSServer) refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, h := range s.mu.handles {
		s.responder.Remove(h)
	}
	s.mu.handles = s.mu.handles[:0]

	for _, def := range s.cfg.Get() {
		svc, err := dnssd.NewService(dnssd.Config{
			Name:   def.Name,
			Type:   def.Service,
			Domain: "local",
			Host:   def.Host,
			Text:   def.TXT,
			IPs:    def.IP,
			Port:   def.Port,
			Ifaces: []string{s.iface},
		})
		if err == nil {
			var h dnssd.ServiceHandle
			if h, err = s.responder.Add(svc); err == nil {
				s.mu.handles = append(s.mu.handles, h)
			}
		}
		if err == nil {
			s.logger.Info("registered service",
				"host", def.Host, "port", def.Port, "ip", def.IP, "name", def.Name)
		} else {
			s.logger.Warn("failed to register mDNS service", "err", err,
				"host", def.Host, "port", def.Port, "ip", def.IP, "name", def.Name)
		}
	}
}
