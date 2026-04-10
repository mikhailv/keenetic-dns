package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/cache" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	configFile := flag.String("config", "./config.yaml", "config file path")
	pprofAddr := flag.String("pprof", "", "pprof handler address")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		exitWithError(fmt.Errorf("failed to load config: %w", err))
	}

	logger, logStream, logFlush := setupLogger(*debug, cfg.History.LogSize)
	defer logFlush()
	defer util.RunPeriodically(ctx.Done(), 10*time.Second, logFlush).Wait()

	// TODO: revert after deadlock investigation
	// defer setup.Pprof(*pprofAddr, logger)()
	setup.Pprof(*pprofAddr, logger)

	routingCfg := config.NewDynamic(&cfg.Routing)
	mdnsServicesCfg := config.NewDynamic(cfg.MDNS.Services)

	logger.Info("config loaded", "route_timeout", cfg.Routing.RouteTimeout)

	dnsStore, dnsStoreSave := setupDNSStore(cfg.Storage.Local.File, log.WithPrefix(logger, "dns_store"), cfg.Routing.RouteTimeout)
	defer dnsStoreSave()
	defer util.RunPeriodically(ctx.Done(), cfg.Storage.Local.SaveInterval, dnsStoreSave).Wait()

	networkService, err := agentclient.NewNetworkServiceClient(cfg.Agent.BaseURL, cfg.Agent.Timeout)
	if err != nil {
		exitWithError(fmt.Errorf("failed to create agent client: %w", err))
	}

	ipRoutes := NewIPRouteController(routingCfg, log.WithPrefix(logger, "routes"), dnsStore, networkService, cfg.Routing.RouteTimeout)
	ipRoutes.Start(ctx)

	conntrackStore := conntrack.NewFileStore(cfg.Conntrack.DataDir)
	defer closeCloser(conntrackStore, "conntrack store", logger)

	conntrackStream := stream.NewBufferedStream[conntrack.Bucket](cfg.History.ConntrackSize)

	conntrackTracker := conntrack.NewTracker(conntrack.TrackerConfig{
		PollInterval:   cfg.Conntrack.PollInterval,
		BucketInterval: cfg.Conntrack.BucketInterval,
		ChunkInterval:  cfg.Conntrack.ChunkInterval,
		CacheDuration:  cfg.Conntrack.CacheDuration,
		SaveInterval:   cfg.Conntrack.SaveInterval,
	}, log.WithPrefix(logger, "conntrack"), networkService, conntrackStore, conntrackStream)
	defer conntrackTracker.Start(ctx).Wait()

	dnsCache, dnsCacheSave := setupDNSCache("dns_cache.dat", log.WithPrefix(logger, "dns_cache"))
	defer closeCloser(dnsCache, "dns cache", logger)
	defer dnsCacheSave()

	defer util.RunPeriodically(ctx.Done(), 10*time.Minute, dnsCacheSave).Wait()

	dnsQueryStream := stream.NewBufferedStream[types.DNSQuery](cfg.History.DNSQuerySize)
	rawQueryStream := stream.NewBufferedStream[types.DNSRawQuery](cfg.History.DNSQuerySize)

	dnsLogger := log.WithPrefix(logger, "dns")
	dnsQueryStream.Listen(func(cursor stream.Cursor, query types.DNSQuery) {
		dnsLogger.Debug("domain resolved", "domain", query.Domain, "ips", len(query.IPs), "client_addr", query.ClientAddr)
	})

	resolver, err := createResolver(cfg.DNS.Providers, logger)
	if err != nil {
		exitWithError(fmt.Errorf("failed to create resolver: %w", err))
	}

	settableResolver := NewSettableResolver(resolver)
	defer closeCloser(settableResolver, "resolver", logger)

	defer watchConfigUpdate(ctx, logger, *configFile, 5*time.Second, func(cfg config.Config) {
		routingCfg.Set(&cfg.Routing)
		mdnsServicesCfg.Set(cfg.MDNS.Services)
		if newResolver, err := createResolver(cfg.DNS.Providers, logger); err != nil {
			logger.Error("failed to create resolver after config update", "err", err)
		} else if err := settableResolver.SetResolver(newResolver); err != nil {
			logger.Error("failed to close resolver", "err", err)
		}
	}).Wait()

	resolver = NewMiddlewareChainResolver(
		[]Middleware{
			NewRawQueryMiddleware(rawQueryStream),                // pre+post
			NewTTLOverrideMiddleware(cfg.DNS.TTLOverride),        // post
			EnableMiddleware(DropECHMiddleware, cfg.DNS.DropECH), // post
			SingleInflightMiddleware,                             // pre
			NewIPRoutingMiddleware(dnsStore, ipRoutes, dnsQueryStream, log.WithPrefix(logger, "ip_routing")), // post
			ErrorSafeResponseMiddleware, // post
		},
		NewCachedResolver("cache", settableResolver, dnsCache),
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
		conntrackTracker,
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
	logger.Info("shutting down...")
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
	logger.Info("closing " + name + "...")
	if err := closer.Close(); err != nil {
		logger.Error("failed to close "+name, "err", err)
	} else {
		logger.Info("closed " + name)
	}
}

func setupDNSStore(file string, logger *slog.Logger, retentionTime time.Duration) (store *DNSStore, save func()) {
	store = NewDNSStore(retentionTime)

	logger = logger.With("file", file)

	removeExpired := func() {
		removed, dur, _ := measure(func() ([]*types.DomainLookup, error) { return store.RemoveExpired(), nil })
		if len(removed) > 0 {
			if logger.Enabled(context.Background(), slog.LevelDebug) {
				for _, r := range removed {
					logger.Debug("dns record expired", "domain", r.Domain, "ips", r.IPs, "resolved", r.Time)
				}
			}
			logger.Info("removed expired records", "removed", len(removed), "duration", dur)
		}
	}

	loadFromFile(file, logger, store.Load)

	return store, func() {
		removeExpired()
		saveToFile(file, logger, store.Save)
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

func setupDNSCache(file string, logger *slog.Logger) (cache DNSCache, save func()) {
	cache = NewMemoryDNSCache()
	pcache, _ := cache.(PersistentDNSCache)
	if pcache == nil {
		return cache, func() {}
	}

	logger = logger.With("file", file)

	loadFromFile(file, logger, pcache.Load)

	return cache, func() {
		saveToFile(file, logger, pcache.Save)
	}
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

func watchConfigUpdate(
	ctx context.Context,
	logger *slog.Logger,
	configFile string,
	updateCheckInterval time.Duration,
	onUpdate func(cfg config.Config),
) util.Waiter {
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
	return util.RunPeriodically(ctx.Done(), updateCheckInterval, func() {
		if t, ok := getModTime(); ok && t.After(modTime) {
			if reloadConfig() {
				modTime = t
			}
		}
	})
}

func createDNSProvider(name string, cfg config.DNSProvider) (Provider, error) {
	if (cfg.Endpoint == nil) == (len(cfg.Hosts) == 0) {
		return nil, fmt.Errorf("exactly one of 'endpoint' or 'hosts' property for DNS provider %q must be provided", name)
	}

	var resolver Resolver
	if len(cfg.Hosts) > 0 {
		resolver = NewStaticHostResolver(name+" (static)", cfg.Hosts, time.Minute)
	} else {
		switch cfg.Endpoint.Scheme {
		case "http", "https":
			resolver = NewDoHClient(name+" (DoH)", cfg.Endpoint.String(), cfg.Timeout)
		case "dns", "dns+udp":
			resolver = NewDNSClient(name+" (udp)", "udp", cfg.Endpoint.Host, cfg.Timeout)
		case "dns+tcp":
			resolver = NewDNSClient(name+" (tcp)", "tcp", cfg.Endpoint.Host, cfg.Timeout)
		case "mdns":
			resolver = NewMDNSClient(name+" (mDNS)", cfg.Endpoint.Host, cfg.Timeout)
		}
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

	return nil, errors.New("no suitable network interface found")
}

func loadFromFile(file string, logger *slog.Logger, loader func(io.Reader) (int, error)) {
	if f, err := os.Open(file); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Error("failed to open file", "err", err)
		}
	} else if loaded, dur, err := measure(func() (int, error) { return loader(f) }); err != nil {
		_ = f.Close()
		logger.Error("failed to load from file", "err", err)
	} else {
		_ = f.Close()
		logger.Info("loaded from file", "records", loaded, "duration", dur)
	}
}

func saveToFile(file string, logger *slog.Logger, saver func(io.Writer) (int, error)) {
	syncClose := func(f *os.File) {
		if err := f.Sync(); err != nil {
			logger.Error("failed to sync file", "err", err)
		}
		if err := f.Close(); err != nil {
			logger.Error("failed to close file", "err", err)
		}
	}
	if f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644); err != nil {
		logger.Error("failed to create file", "err", err)
	} else if saved, dur, err := measure(func() (int, error) { return saver(f) }); err != nil {
		logger.Error("failed to save to file", "err", err)
		syncClose(f)
	} else {
		logger.Info("saved to file", "records", saved, "duration", dur)
		syncClose(f)
	}
}

func measure[T any](fn func() (T, error)) (T, time.Duration, error) {
	st := time.Now()
	r, err := fn()
	return r, time.Since(st).Truncate(time.Microsecond), err
}
