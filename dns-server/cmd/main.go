package main

import (
	"context"
	"flag"
	"fmt"
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

func main() {
	ctx := setup.ListenStopSignal(context.Background())

	configFile := flag.String("config", "./config.yaml", "config file path")
	pprofAddr := flag.String("pprof", "", "pprof handler address")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	cfg, err := LoadConfig(*configFile)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to load config: %v", err)
		os.Exit(1)
	}

	logger, logStream := setupLogger(*debug, cfg.LogHistorySize)

	setup.Pprof(ctx, *pprofAddr, logger)

	dnsStore := NewDNSStore()
	saveStore := initDNSStore(cfg.Dump.File, log.WithPrefix(logger, "dns_store"), dnsStore)
	go util.RunPeriodically(ctx, cfg.Dump.Interval, func(ctx context.Context) { saveStore() })

	networkService := agent.NewNetworkServiceClient(cfg.AgentBaseURL, cfg.AgentTimeout)

	ipRoutes := NewIPRouteController(cfg.Routing, log.WithPrefix(logger, "routes"), dnsStore, networkService, cfg.ReconcileInterval, cfg.ReconcileTimeout)
	ipRoutes.Start(ctx)

	listenConfigUpdate(logger, *configFile, 5*time.Second, func(cfg Config) {
		ipRoutes.UpdateConfig(ctx, cfg.Routing.RoutingDynamicConfig)
	})

	var dnsProvider DNSResolver
	if strings.HasPrefix(cfg.DNSProvider, "http") {
		dnsProvider = NewDoHClient(cfg.DNSProvider, cfg.DNSProviderTimeout)
	} else {
		dnsProvider = NewDNSClient(cfg.DNSProvider, cfg.DNSProviderTimeout)
	}
	dnsProvider = NewMDNSResolver(dnsProvider, cfg.MDNS)

	dnsCache := NewDNSCache()
	go util.RunPeriodically(ctx, time.Minute, func(ctx context.Context) { dnsCache.RemoveExpired() })

	service := NewDNSRoutingService(log.WithPrefix(logger, "dns_svc"), dnsProvider, dnsStore, ipRoutes, cfg.DNSQueryHistorySize)

	resolver := NewSingleInflightDNSResolver(service)
	resolver = NewCachedDNSResolver(resolver, dnsCache)
	resolver = NewTTLOverridingDNSResolver(resolver, cfg.DNSTTLOverride)

	httpServer := NewHTTPServer(cfg.HTTPAddr, log.WithPrefix(logger, "http"), resolver, ipRoutes, logStream, service.QueryStream(), service.RawQueryStream())
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

func setupLogger(debug bool, historySize int) (*slog.Logger, *stream.Buffered[log.Entry]) {
	var recorder log.Recorder
	logger := setup.Logger(debug, func(handler slog.Handler) slog.Handler {
		recorder = log.NewRecorder(handler, historySize)
		return recorder
	})
	return logger, recorder.Stream()
}

func listenConfigUpdate(logger *slog.Logger, configFile string, updateCheckInterval time.Duration, onUpdate func(cfg Config)) {
	getModTime := func() (time.Time, bool) {
		f, err := os.Stat(configFile)
		if err != nil {
			return time.Time{}, false
		}
		return f.ModTime(), true
	}

	reloadConfig := func() bool {
		if cfg, err := LoadConfig(configFile); err != nil {
			logger.Error("failed to load config", "err", err)
			return false
		} else {
			logger.Info("config change detected")
			onUpdate(*cfg)
			return true
		}
	}

	modTime, _ := getModTime()

	go func() {
		for range time.Tick(updateCheckInterval) {
			if t, ok := getModTime(); ok && t.After(modTime) {
				if reloadConfig() {
					modTime = t
				}
			}
		}
	}()
}
