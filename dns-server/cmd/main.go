package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/mikhailv/keenetic-dns/agent"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal" //nolint:stylecheck //ignore
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/setup"
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	logBufferSize = 2_000
)

func main() {
	ctx := setup.ListenStopSignal(context.Background())

	var pprofAddr string
	var debug bool

	flag.StringVar(&pprofAddr, "pprof", "", "pprof handler address")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.Parse()

	logger, logStream := setupLogger(debug)

	setup.Pprof(ctx, pprofAddr, logger)

	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	dnsStore := NewDNSStore()
	saveStore := initDNSStore(cfg.Dump.File, log.WithPrefix(logger, "dns_store"), dnsStore)
	go util.RunPeriodically(ctx, cfg.Dump.Interval, func(ctx context.Context) { saveStore() })

	networkService := agent.NewNetworkServiceClient(cfg.AgentBaseURL, cfg.AgentTimeout)

	ipRoutes := NewIPRouteController(cfg.Routing, log.WithPrefix(logger, "routes"), dnsStore, networkService, cfg.ReconcileInterval, cfg.ReconcileTimeout)
	ipRoutes.Start(ctx)

	var dnsProvider DNSResolver
	if strings.HasPrefix(cfg.DNSProvider, "http") {
		dnsProvider = NewDoHClient(cfg.DNSProvider, cfg.DNSProviderTimeout)
	} else {
		dnsProvider = NewDNSClient(cfg.DNSProvider, cfg.DNSProviderTimeout)
	}

	dnsCache := NewDNSCache()
	go util.RunPeriodically(ctx, time.Minute, func(ctx context.Context) { dnsCache.RemoveExpired() })

	service := NewDNSRoutingService(log.WithPrefix(logger, "dns_svc"), dnsProvider, dnsStore, ipRoutes)

	resolver := NewSingleInflightDNSResolver(service)
	resolver = NewCachedDNSResolver(resolver, dnsCache, cfg.DNSTTLOverride)

	httpServer := NewHTTPServer(cfg.Addr, log.WithPrefix(logger, "http"), resolver, ipRoutes, logStream, service.ResolveStream())
	go httpServer.Serve(ctx)

	udpServer := NewDNSServer(cfg.Addr, log.WithPrefix(logger, "dns"), resolver)
	go udpServer.Serve(ctx)

	<-ctx.Done()

	saveStore()
}

func initDNSStore(file string, logger *slog.Logger, store *DNSStore) (save func()) {
	logger = logger.With("file", file)
	if err := store.Load(file); err != nil {
		logger.Error("failed to load", "err", err)
	}
	return func() {
		logger.Info("saving ...")
		if err := store.Save(file); err != nil {
			logger.Error("failed to save", "err", err)
		}
	}
}

func setupLogger(debug bool) (*slog.Logger, *stream.Buffered[log.Entry]) {
	var recorder log.Recorder
	logger := setup.Logger(debug, func(handler slog.Handler) slog.Handler {
		recorder = log.NewRecorder(handler, logBufferSize)
		return recorder
	})
	return logger, recorder.Stream()
}