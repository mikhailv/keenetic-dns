package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/mikhailv/keenetic-dns/agent"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/cache" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"            //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/middleware" //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/resolvers"  //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/routing"           //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/server"            //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/storage"           //nolint:staticcheck //ignore
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
		exitWithError(fmt.Errorf("failed to load config: %w", err))
	}

	logger, logStream, logFlush := setupLogger(*debug, cfg.History.LogSize)
	go util.RunPeriodically(ctx.Done(), 10*time.Second, logFlush)
	defer logFlush()

	setup.Pprof(ctx, *pprofAddr, logger)

	routingCfg := config.NewDynamic(&cfg.Routing)
	mdnsServicesCfg := config.NewDynamic(cfg.MDNS.Services)

	resolver, err := createResolver(cfg.DNS.Providers, logger)
	if err != nil {
		exitWithError(fmt.Errorf("failed to create resolver: %w", err))
	}

	settableResolver := NewSettableResolver(resolver)
	defer closeCloser(settableResolver, "resolver", logger)

	listenConfigUpdate(ctx, logger, *configFile, 5*time.Second, func(cfg config.Config) {
		routingCfg.Set(&cfg.Routing)
		mdnsServicesCfg.Set(cfg.MDNS.Services)
		if newResolver, err := createResolver(cfg.DNS.Providers, logger); err != nil {
			logger.Error("failed to create resolver after config update", "err", err)
		} else if err := settableResolver.SetResolver(newResolver); err != nil {
			logger.Error("failed to close resolver", "err", err)
		}
	})

	logger.Info("config loaded", "route_timeout", cfg.Routing.RouteTimeout)

	dnsStore := NewDNSStore(cfg.Routing.RouteTimeout)
	saveStore := createDNSStoreSaver(cfg.Storage.Local.File, log.WithPrefix(logger, "dns_store"), dnsStore)
	go util.RunPeriodically(ctx.Done(), cfg.Storage.Local.SaveInterval, saveStore)

	networkService := agent.NewNetworkServiceClient(cfg.Agent.BaseURL, cfg.Agent.Timeout)

	ipRoutes := NewIPRouteController(routingCfg, log.WithPrefix(logger, "routes"), dnsStore, networkService)
	ipRoutes.Start(ctx)

	dnsCache := NewMemoryDNSCache()
	defer closeCloser(dnsCache, "dns cache", logger)

	dnsQueryStream := stream.NewBufferedStream[types.DNSQuery](cfg.History.DNSQuerySize)
	rawQueryStream := stream.NewBufferedStream[types.DNSRawQuery](cfg.History.DNSQuerySize)

	dnsLogger := log.WithPrefix(logger, "dns")
	dnsQueryStream.Listen(func(cursor stream.Cursor, query types.DNSQuery) {
		dnsLogger.Debug("domain resolved", "domain", query.Domain, "ips", len(query.IPs), "client_addr", query.ClientAddr)
	})

	resolver = NewMiddlewareChainResolver(
		[]Middleware{
			EnableMiddleware(VerboseMiddleware, *verbose),              // pre+post
			NewRawQueryMiddleware(rawQueryStream),                      // pre+post
			NewTTLOverrideMiddleware(cfg.DNS.TTLOverride),              // post
			SingleInflightMiddleware,                                   // pre
			NewIPRoutingMiddleware(dnsStore, ipRoutes, dnsQueryStream), // post
			NewCacheMiddleware(dnsCache),                               // pre
			ErrorSafeResponseMiddleware,                                // post
		},
		settableResolver,
	)

	httpServer := NewHTTPServer(
		cfg.HTTPAddr,
		log.WithPrefix(logger, "http"),
		resolver,
		ipRoutes,
		networkService,
		logStream,
		dnsQueryStream,
		rawQueryStream,
	)
	go serve(ctx, httpServer)

	udpServer := NewDNSServer(cfg.Addr, dnsLogger, resolver)
	go serve(ctx, udpServer)

	if cfg.MDNS.Enabled {
		iface, err := getDefaultInterface()
		exitIfError(err)
		mdnsServer := NewMDNSServer(log.WithPrefix(logger, "mdns"), iface.Name, mdnsServicesCfg)
		go serve(ctx, mdnsServer)
	}

	logFlush()

	<-ctx.Done()

	saveStore()
}

func serve(ctx context.Context, server interface{ Serve(context.Context) error }) {
	exitIfError(server.Serve(ctx))
}

func exitWithError(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func exitIfError(err error) {
	if err != nil {
		exitWithError(err)
	}
}

func closeCloser(closer io.Closer, name string, logger *slog.Logger) {
	if err := closer.Close(); err != nil {
		logger.Error("failed to close "+name, "err", err)
	}
}

func createDNSStoreSaver(file string, logger *slog.Logger, store *DNSStore) (doSave func()) {
	logger = logger.With("file", file)

	load := func() {
		logger.Info("loading ...")
		count, dur, err := measure(func() (int, error) { return store.Load(file) })
		if err != nil {
			logger.Error("load failed", "err", err)
		} else {
			logger.Info("load succeeded", "records", count, "duration", dur)
		}
	}

	save := func() {
		logger.Info("saving ...")
		count, dur, err := measure(func() (int, error) { return store.Save(file) })
		if err != nil {
			logger.Error("save failed", "err", err)
		} else {
			logger.Info("save succeeded", "records", count, "duration", dur)
		}
	}

	removeExpired := func() {
		removed, dur, _ := measure(func() ([]types.DNSRecord, error) { return store.RemoveExpired(), nil })
		if len(removed) > 0 {
			if logger.Enabled(context.Background(), slog.LevelDebug) {
				for _, r := range removed {
					logger.Debug("dns record expired", "domain", r.Domain, "ip", r.IP, "resolved", r.Resolved)
				}
			}
			logger.Info("removed expired records", "removed", len(removed), "duration", dur)
		}
	}

	load()
	return func() {
		removeExpired()
		save()
	}
}

func createResolver(providersConfig map[string]config.DNSProvider, logger *slog.Logger) (Resolver, error) {
	providers := make([]Provider, 0, len(providersConfig))
	for name, c := range providersConfig {
		if c.Enabled {
			p, err := createDNSProvider(name, c)
			if err != nil {
				return nil, fmt.Errorf("failed to create DNS provider %q: %w", name, err)
			}
			providers = append(providers, p)
			logger.Info("DNS provider registered", "name", name, "endpoint", c.Endpoint.String())
		}
	}
	return NewMultiProviderResolver(providers), nil
}

func setupLogger(debug bool, historySize int) (logger *slog.Logger, stream *stream.Buffered[log.Entry], flush func()) {
	logger = setup.Logger(debug, func(handler slog.Handler) slog.Handler {
		buffered := log.NewBufferedHandler(handler, 300)
		flush = buffered.Flush
		recorder := log.NewRecorder(buffered, historySize)
		stream = recorder.Stream()
		return log.NewPrefixHandler(recorder)
	})
	return logger, stream, flush
}

func listenConfigUpdate(ctx context.Context, logger *slog.Logger, configFile string, updateCheckInterval time.Duration, onUpdate func(cfg config.Config)) {
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

	go func() {
		modTime, _ := getModTime()
		util.RunPeriodically(ctx.Done(), updateCheckInterval, func() {
			if t, ok := getModTime(); ok && t.After(modTime) {
				if reloadConfig() {
					modTime = t
				}
			}
		})
	}()
}

func createDNSProvider(name string, cfg config.DNSProvider) (Provider, error) {
	if (cfg.Endpoint == nil) == (len(cfg.Hosts) == 0) {
		return nil, fmt.Errorf("exactly one of 'endpoint' or 'hosts' property for DNS provider %q must be provided", name)
	}

	var resolver Resolver
	if len(cfg.Hosts) > 0 {
		resolver = NewStaticHostResolver(cfg.Hosts, time.Minute)
	} else {
		switch cfg.Endpoint.Scheme {
		case "http", "https":
			resolver = NewDoHClient(name, cfg.Endpoint.String(), cfg.Timeout)
		case "dns", "dns+udp":
			resolver = NewDNSClient(name, "udp", cfg.Endpoint.Host, cfg.Timeout)
		case "dns+tcp":
			resolver = NewDNSClient(name, "tcp", cfg.Endpoint.Host, cfg.Timeout)
		case "mdns":
			resolver = NewMDNSClient(name, cfg.Endpoint.Host, cfg.Timeout)
		}
	}
	if cfg.DropECH {
		resolver = NewMiddlewareChainResolver([]Middleware{DropECHMiddleware}, resolver)
	}
	return NewProvider(resolver, cfg)
}

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

func measure[T any](fn func() (T, error)) (T, time.Duration, error) {
	st := time.Now()
	r, err := fn()
	return r, time.Since(st), err
}
