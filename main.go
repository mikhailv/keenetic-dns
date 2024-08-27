package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
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
		logger.Error("error loading config", slog.Any("err", err))
		os.Exit(1)
	}

	dnsStore := NewDNSStore()
	if err := dnsStore.Load(cfg.Dump.File); err != nil {
		logger.Error("error loading dns store", slog.Any("err", err))
	}

	go func() {
		util.RunPeriodically(ctx, cfg.Dump.Interval, func(ctx context.Context) {
			dumpStore(dnsStore, cfg.Dump.File, logger)
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

	service := NewDNSRoutingService(&cfg.Routing, dnsProvider, ipRoutes)
	resolver := NewSingleInflightDNSResolver(service)

	httpServer := NewHTTPServer(cfg.HTTPAddr, logger, cfg, resolver, ipRoutes, logStream, service.ResolveStream())
	go httpServer.Serve(ctx)

	udpServer := NewDNSServer(cfg.UDPAddr, logger, cfg, resolver)
	go udpServer.Serve(ctx)

	<-ctx.Done()

	dumpStore(dnsStore, cfg.Dump.File, logger)
}

func dumpStore(store *DNSStore, file string, logger *slog.Logger) {
	logger.Info("saving dns store...", slog.Any("file", file))
	if err := store.Save(file); err != nil {
		logger.Error("error saving dns store", slog.Any("err", err))
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
