package main

import (
	"context"
	"flag"

	"github.com/mikhailv/keenetic-dns/agent/internal"
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/setup"
)

func main() {
	ctx := setup.ListenStopSignal(context.Background())

	var httpServerAddr string
	var pprofAddr string
	var debug bool

	flag.StringVar(&httpServerAddr, "addr", "0.0.0.0:5332", "http server address")
	flag.StringVar(&pprofAddr, "pprof", "", "pprof handler address")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.Parse()

	logger := setup.Logger(debug, nil)

	setup.Pprof(ctx, pprofAddr, logger)

	networkService := internal.NewNetworkService(log.WithPrefix(logger, "network_svc"))

	httpServer := internal.NewHTTPServer(httpServerAddr, log.WithPrefix(logger, "http"), networkService)
	go httpServer.Serve(ctx)

	<-ctx.Done()
}
