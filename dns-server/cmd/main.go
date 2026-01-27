package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/mikhailv/keenetic-dns/agent"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/cache" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"            //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/middleware" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/service"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/routing" //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/server"  //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/storage" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/setup"
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

func main() { //nolint:funlen // ignore
	ctx := setup.ListenStopSignal(context.Background())

	configFile := flag.String("config", "./config.yaml", "config file path")
	pprofAddr := flag.String("pprof", "", "pprof handler address")
	debug := flag.Bool("debug", false, "enable debug logging")
	verbose := flag.Bool("verbose", false, "enable verbose output")
	flag.Parse()

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		exitIfError(fmt.Errorf("failed to load config: %w", err))
	}

	logger, logStream, logFlush := setupLogger(*debug, cfg.History.LogSize)
	defer logFlush()

	setup.Pprof(ctx, *pprofAddr, logger)

	routingCfg := config.NewDynamic(&cfg.Routing)
	hostsCfg := config.NewDynamic(cfg.DNS.Hosts)
	mdnsServicesCfg := config.NewDynamic(cfg.MDNS.Services)

	listenConfigUpdate(logger, *configFile, 5*time.Second, func(cfg config.Config) {
		routingCfg.Set(&cfg.Routing)
		hostsCfg.Set(cfg.DNS.Hosts)
		mdnsServicesCfg.Set(cfg.MDNS.Services)
	})

	dnsStore := NewDNSStore()
	saveStore := newDNSStoreSaver(cfg.Storage.Local.File, log.WithPrefix(logger, "dns_store"), dnsStore)
	go util.RunPeriodically(ctx, cfg.Storage.Local.SaveInterval, func(ctx context.Context) { saveStore() })

	networkService := agent.NewNetworkServiceClient(cfg.Agent.BaseURL, cfg.Agent.Timeout)

	ipRoutes := NewIPRouteController(routingCfg, log.WithPrefix(logger, "routes"), dnsStore, networkService)
	ipRoutes.Start(ctx)

	dnsCache := NewDNSCache()
	go util.RunPeriodically(ctx, time.Minute, func(ctx context.Context) { dnsCache.RemoveExpired() })

	dnsQueryStream := stream.NewBufferedStream[types.DNSQuery](cfg.History.DNSQuerySize)
	rawQueryStream := stream.NewBufferedStream[types.DNSRawQuery](cfg.History.DNSQuerySize)

	providers := make([]Provider, 0, len(cfg.DNS.Providers))
	for name, c := range cfg.DNS.Providers {
		if c.Enabled {
			providers = append(providers, createDNSProvider(name, c))
			logger.Info("DNS provider registered", slog.String("name", name), slog.String("endpoint", c.Endpoint.String()))
		}
	}

	resolver := NewMultiProviderResolver(providers)

	svc := service.NewDNSRoutingService(log.WithPrefix(logger, "dns_svc"), resolver, dnsStore, ipRoutes, dnsQueryStream, rawQueryStream)

	handler := NewMiddlewareChainHandler([]Middleware{
		EnableMiddleware(VerboseMiddleware, *verbose),
		SingleInflightMiddleware,
		NewStaticHostsMiddleware(hostsCfg, time.Minute),
		NewCacheMiddleware(dnsCache),
		NewTTLOverrideMiddleware(cfg.DNS.TTLOverride),
		ErrorSafeResponseMiddleware,
	}, svc.Resolve)

	httpServer := NewHTTPServer(
		cfg.HTTPAddr,
		log.WithPrefix(logger, "http"),
		ResolverFunc(handler),
		ipRoutes,
		networkService,
		logStream,
		dnsQueryStream,
		rawQueryStream,
	)
	go serve(ctx, httpServer)

	udpServer := NewDNSServer(cfg.Addr, log.WithPrefix(logger, "dns"), ResolverFunc(handler))
	go serve(ctx, udpServer)

	if cfg.MDNS.Enabled {
		iface, err := getDefaultInterface()
		exitIfError(err)
		mdnsServer := NewMDNSServer(log.WithPrefix(logger, "mdns"), iface.Name, mdnsServicesCfg)
		go serve(ctx, mdnsServer)
	}

	<-ctx.Done()

	saveStore()
}

func serve(ctx context.Context, server interface{ Serve(context.Context) error }) {
	exitIfError(server.Serve(ctx))
}

func exitIfError(err error) {
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newDNSStoreSaver(file string, logger *slog.Logger, store *DNSStore) (save func()) {
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

func setupLogger(debug bool, historySize int) (logger *slog.Logger, stream *stream.Buffered[log.Entry], flush func()) {
	logger = setup.Logger(debug, func(handler slog.Handler) slog.Handler {
		buffered := log.NewBufferedHandler(handler, 300, 10*time.Second)
		flush = buffered.Flush
		recorder := log.NewRecorder(buffered, historySize)
		stream = recorder.Stream()
		return log.NewPrefixHandler(recorder)
	})
	return logger, stream, flush
}

func listenConfigUpdate(logger *slog.Logger, configFile string, updateCheckInterval time.Duration, onUpdate func(cfg config.Config)) {
	getModTime := func() (time.Time, bool) {
		f, err := os.Stat(configFile)
		if err != nil {
			return time.Time{}, false
		}
		return f.ModTime(), true
	}

	reloadConfig := func() bool {
		if cfg, err := config.LoadConfig(configFile); err != nil {
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

func createDNSProvider(name string, cfg config.DNSProvider) Provider {
	var client Resolver
	switch cfg.Endpoint.Scheme {
	case "http", "https":
		client = NewDoHClient(name, cfg.Endpoint.String(), cfg.Timeout)
	case "dns", "dns+udp":
		client = NewDNSClient(name, "udp", cfg.Endpoint.Host, cfg.Timeout)
	case "dns+tcp":
		client = NewDNSClient(name, "tcp", cfg.Endpoint.Host, cfg.Timeout)
	case "mdns":
		client = NewMDNSClient(name, cfg.Endpoint.Host, cfg.Timeout)
	}
	if cfg.DropECH {
		client = ResolverFunc(DropECHMiddleware(client.Resolve))
	}
	return NewProvider(client, cfg)
}

//nolint:cyclop // ignore complexity
func getDefaultInterface() (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagRunning == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addresses, _ := iface.Addrs()
		for _, addr := range addresses {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("no suitable network interface found")
}
