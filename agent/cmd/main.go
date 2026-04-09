package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mikhailv/keenetic-dns/agent/internal"
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/setup"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var httpServerAddr string
	var pprofAddr string
	var debug bool

	flag.StringVar(&httpServerAddr, "addr", "0.0.0.0:5332", "http server address")
	flag.StringVar(&pprofAddr, "pprof", "", "pprof handler address")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.Parse()

	logger, logFlush := setupLogger(debug)
	defer util.RunPeriodically(ctx.Done(), 10*time.Second, logFlush).Wait()
	defer logFlush()

	setup.Pprof(ctx, pprofAddr, logger)

	networkService := internal.NewNetworkService(log.WithPrefix(logger, "network_svc"))

	httpServer := internal.NewHTTPServer(httpServerAddr, log.WithPrefix(logger, "http"), networkService)
	go serve(ctx, httpServer)

	<-ctx.Done()
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

func setupLogger(debug bool) (logger *slog.Logger, flush func()) {
	logger = setup.Logger(debug, func(handler slog.Handler) slog.Handler {
		buffered := log.NewBufferedHandler(handler, 300)
		flush = buffered.Flush
		return log.NewPrefixHandler(buffered)
	})
	return logger, flush
}
