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
	"github.com/mikhailv/keenetic-dns/dns-server/internal/blocklist"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/blockstats"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/cache" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/handlers"
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/middleware" //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc/resolvers"  //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/routing"           //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/server"            //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/dns-server/internal/storage"           //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/log"
	. "github.com/mikhailv/keenetic-dns/internal/setup" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

func main() { //nolint:funlen // ignore
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	configFile := flag.String("config", "./config.yaml", "config file path")
	pprofAddr := flag.String("pprof", "", "pprof handler address")
	flag.Parse()

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		ExitWithError(fmt.Errorf("failed to load config: %w", err))
	}
	if err = cfg.Validate(); err != nil {
		ExitWithError(fmt.Errorf("invalid config: %w", err))
	}

	logger, logStream, logFlush := LoggerStream(cfg.Logging.Debug, 300, cfg.Logging.HistorySize)
	defer LogPanic(logger)
	defer logFlush()
	defer util.RunPeriodically(ctx.Done(), 10*time.Second, logFlush).Wait()

	defer Pprof(*pprofAddr, logger)()

	routingCfg := util.NewDynamic(&cfg.Routing)
	mdnsServicesCfg := util.NewDynamic(cfg.MDNS.Services)

	logger.Info("config loaded", "route_timeout", cfg.Routing.RouteTimeout)

	dnsStore, dnsStoreSave := setupDNSStore(cfg.Storage.Local.File, log.WithPrefix(logger, "dns_store"), cfg.Routing.RouteTimeout)
	defer dnsStoreSave()
	defer util.RunPeriodically(ctx.Done(), cfg.Storage.Local.SaveInterval, dnsStoreSave).Wait()

	networkService, err := agentclient.NewNetworkServiceClient(cfg.Agent.BaseURL, cfg.Agent.Timeout)
	if err != nil {
		ExitWithError(fmt.Errorf("failed to create agent client: %w", err))
	}

	ipRoutes := NewIPRouteController(routingCfg, log.WithPrefix(logger, "routes"), dnsStore, networkService, cfg.Routing.RouteTimeout)
	ipRoutes.Start(ctx)

	conntrackStore := conntrack.NewFileStore(cfg.Conntrack.DataDir, log.WithPrefix(logger, "conntrack.filestore"))
	if err = conntrackStore.Init(ctx); err != nil {
		ExitWithError(fmt.Errorf("failed to init conntrack store: %w", err))
	}
	defer closeCloser(conntrackStore, "conntrack store", logger)

	conntrackTracker := conntrack.NewTracker(conntrack.TrackerConfig{
		PollInterval:   cfg.Conntrack.PollInterval,
		BucketInterval: cfg.Conntrack.BucketInterval,
		ChunkInterval:  cfg.Conntrack.ChunkInterval,
		CacheDuration:  cfg.Conntrack.CacheDuration,
		SaveInterval:   cfg.Conntrack.SaveInterval,
	}, log.WithPrefix(logger, "conntrack"), networkService, conntrackStore)
	defer conntrackTracker.Start(ctx).Wait()

	dnsCache, dnsCacheSave := setupDNSCache("dns_cache.dat", log.WithPrefix(logger, "dns_cache"))
	defer closeCloser(dnsCache, "dns cache", logger)
	defer dnsCacheSave()

	defer util.RunPeriodically(ctx.Done(), 10*time.Minute, dnsCacheSave).Wait()

	var blockingMiddleware Middleware

	if cfg.Blocking.Enabled {
		blocklistManager := blocklist.NewManager(cfg.Blocking.Config, log.WithPrefix(logger, "blocklist"))
		defer closeCloser(blocklistManager, "blocklist", logger)
		defer blocklistManager.Start(ctx).Wait()

		var blockRecorder handlers.BlockRecorder

		if cfg.Blocking.Stats.Enabled {
			statsStore := blockstats.NewFileStore(cfg.Blocking.Stats.DataDir, log.WithPrefix(logger, "blockstats.filestore"))
			if err = statsStore.Init(ctx); err != nil {
				ExitWithError(fmt.Errorf("failed to init blockstats store: %w", err))
			}
			defer closeCloser(statsStore, "blockstats store", logger)

			statsRecorder := blockstats.NewRecorder(statsStore, cfg.Blocking.Stats, log.WithPrefix(logger, "blockstats"))
			defer closeCloser(statsRecorder, "blockstats", logger)
			defer statsRecorder.Start(ctx).Wait()

			blockRecorder = statsRecorder

			logger.Info("blockstats enabled",
				"dir", cfg.Blocking.Stats.DataDir, "flush_interval", cfg.Blocking.Stats.FlushInterval)
		}

		blockingMiddleware = NewBlockingMiddleware(blocklistManager, cfg.Blocking.Mode, blockRecorder, log.WithPrefix(logger, "blocking"))

		logger.Info("blocking enabled", "lists", len(cfg.Blocking.EnabledLists()),
			"mode", cfg.Blocking.Mode.String(), "groups", len(cfg.Blocking.Groups))
	} else {
		blockingMiddleware = NopMiddleware
		logger.Info("blocking disabled")
	}

	dnsQueryStream := stream.NewBufferedStream[types.DNSQuery](cfg.DNS.QueryHistorySize)
	rawQueryStream := stream.NewBufferedStream[types.DNSRawQuery](cfg.DNS.QueryHistorySize)

	dnsLogger := log.WithPrefix(logger, "dns")
	dnsQueryStream.Listen(func(cursor stream.Cursor, query types.DNSQuery) {
		dnsLogger.Debug("domain resolved", "domain", query.Domain, "ips", len(query.IPs), "client_ip", query.ClientIP)
	})

	resolver, err := createResolver(cfg.DNS.Providers, logger)
	if err != nil {
		ExitWithError(fmt.Errorf("failed to create resolver: %w", err))
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
			NewRawQueryMiddleware(rawQueryStream),                                            // pre+post
			blockingMiddleware,                                                               // pre+post
			NewTTLOverrideMiddleware(cfg.DNS.TTLOverride),                                    // post
			EnableMiddleware(DropECHMiddleware, cfg.DNS.DropECH),                             // post
			EnableMiddleware(DropAAAAMiddleware, cfg.DNS.DropAAAA),                           // post
			NewQueryLogMiddleware(dnsQueryStream),                                            // post
			SingleFlightMiddleware,                                                           // pre
			NewIPRoutingMiddleware(dnsStore, ipRoutes, log.WithPrefix(logger, "ip_routing")), // post
			ErrorSafeResponseMiddleware,                                                      // post
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
	go Serve(ctx, httpServer)

	dnsServer := NewDNSServer(cfg.Addr, dnsLogger, resolver)
	go Serve(ctx, dnsServer)

	if cfg.MDNS.Enabled {
		iface, err := getDefaultInterface()
		ExitIfError(err)
		mdnsServer := NewMDNSServer(log.WithPrefix(logger, "mdns"), iface.Name, mdnsServicesCfg)
		go Serve(ctx, mdnsServer)
	}

	logFlush()

	<-ctx.Done()
	logger.Info("shutting down...")
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
		m := measure()
		removed := store.RemoveExpired()
		if len(removed) > 0 {
			if logger.Enabled(context.Background(), slog.LevelDebug) {
				for _, r := range removed {
					ips := make([]types.IPv4, len(r.IPs))
					for i, v := range r.IPs {
						ips[i] = v.IP
					}
					logger.Debug("dns record expired", "domain", r.Domain, "ips", ips, "resolved", r.Time)
				}
			}
			logger.Info("removed expired records", "removed", len(removed), "duration", m())
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
	m := measure()
	if f, err := os.Open(file); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Error("failed to open file", "err", err)
		}
	} else if loaded, err := loader(f); err != nil {
		_ = f.Close()
		logger.Error("failed to load from file", "err", err)
	} else {
		_ = f.Close()
		logger.Info("loaded from file", "records", loaded, "duration", m())
	}
}

func saveToFile(file string, logger *slog.Logger, saver func(io.Writer) (int, error)) {
	m := measure()
	var saved int
	err := util.SaveToFileFunc(file, func(w io.Writer) error {
		var err error
		saved, err = saver(w)
		return err
	})
	if err != nil {
		logger.Error("failed to save to file", "err", err)
		return
	}
	logger.Info("saved to file", "records", saved, "duration", m())
}

func measure() func() time.Duration {
	st := time.Now()
	return func() time.Duration {
		return time.Since(st).Truncate(time.Microsecond)
	}
}
