package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"
	"time"

	. "github.com/mikhailv/keenetic-dns/agent/internal" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/internal/log"
	. "github.com/mikhailv/keenetic-dns/internal/setup" //nolint:staticcheck //ignore
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

	logger, logFlush := Logger(debug, 300)
	defer LogPanic(logger)
	defer logFlush()
	defer util.RunPeriodically(ctx.Done(), 10*time.Second, logFlush).Wait()

	defer Pprof(pprofAddr, logger)()

	networkService := NewNetworkService(log.WithPrefix(logger, "network_svc"))

	httpServer := NewHTTPServer(httpServerAddr, log.WithPrefix(logger, "http"), networkService)
	go Serve(ctx, httpServer)

	logFlush()

	<-ctx.Done()
	logger.Info("shutting down...")
}
