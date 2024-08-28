package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	pp "runtime/pprof"
	"strings"
	"syscall"
	"time"

	. "github.com/mikhailv/keenetic-dns/internal" //nolint:stylecheck //ignore
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	logBufferSize = 2_000
)

func main() {
	ctx := listenStopSignal(context.Background())

	var debug bool
	var pprofAddr string
	flag.StringVar(&pprofAddr, "pprof", "", "pprof handler address")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.Parse()

	logger, logStream := setupLogger(debug)

	setupPprof(ctx, pprofAddr, logger)

	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		logger.Error("failed to load config", slog.Any("err", err))
		os.Exit(1)
	}

	dnsStore := NewDNSStore()
	saveStore := initDNSStore(cfg.Dump.File, logger, dnsStore)

	go func() {
		util.RunPeriodically(ctx, cfg.Dump.Interval, func(ctx context.Context) {
			saveStore()
		})
	}()

	ipRoutes := NewIPRouteController(cfg.Routing, logger, dnsStore, cfg.ReconcileInterval)
	ipRoutes.Start(ctx)

	var dnsProvider DNSResolver
	if strings.HasPrefix(cfg.DNSProvider, "http") {
		dnsProvider = NewDoHClient(cfg.DNSProvider, cfg.DNSProviderTimeout)
	} else {
		dnsProvider = NewDNSClient(cfg.DNSProvider, cfg.DNSProviderTimeout)
	}

	service := NewDNSRoutingService(&cfg.Routing, logger, dnsProvider, ipRoutes)
	resolver := NewSingleInflightDNSResolver(service)

	httpServer := NewHTTPServer(cfg.Addr, logger, resolver, ipRoutes, logStream, service.ResolveStream())
	go httpServer.Serve(ctx)

	udpServer := NewDNSServer(cfg.Addr, logger, resolver)
	go udpServer.Serve(ctx)

	<-ctx.Done()

	saveStore()
}

func initDNSStore(file string, logger *slog.Logger, store *DNSStore) (save func()) {
	logger = logger.With(slog.String("file", file))
	if err := store.Load(file); err != nil {
		logger.Error("dns_store: failed to load", slog.Any("err", err))
	}
	return func() {
		logger.Info("dns_store: saving ...")
		if err := store.Save(file); err != nil {
			logger.Error("dns_store: failed to save", slog.Any("err", err))
		}
	}
}

func setupLogger(debug bool) (*slog.Logger, *util.BufferedStream[log.Entry]) {
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	recorder := log.NewRecorder(handler, logBufferSize)
	return slog.New(recorder), recorder.Stream()
}

func setupPprof(ctx context.Context, addr string, logger *slog.Logger) {
	if addr == "" {
		return
	}

	var mux http.ServeMux
	mux.HandleFunc("/", pprof.Index)
	mux.HandleFunc("/cmdline", pprof.Cmdline)
	mux.HandleFunc("/profile", pprof.Profile)
	mux.HandleFunc("/symbol", pprof.Symbol)
	mux.HandleFunc("/trace", pprof.Trace)
	for _, p := range pp.Profiles() {
		name := p.Name()
		mux.HandleFunc("/"+name, func(rw http.ResponseWriter, req *http.Request) {
			req.URL.Path = "/debug/pprof/" + name
			pprof.Index(rw, req)
		})
	}

	srv := http.Server{
		Addr:              addr,
		Handler:           &mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("pprof handler started", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil {
			logger.Error("failed to serve pprof handler", slog.Any("err", err))
		}
	}()

	go func() {
		<-ctx.Done()
		_ = srv.Close()
		logger.Info("pprof handler stopped")
	}()
}

func listenStopSignal(parentCtx context.Context) context.Context {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(parentCtx)
	go func() {
		<-signalCh
		cancel()
	}()
	return ctx
}
