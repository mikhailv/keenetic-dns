package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/mikhailv/keenetic-dns/internal/log"
	. "github.com/mikhailv/keenetic-dns/internal/setup"  //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/ipinfo/internal" //nolint:staticcheck //ignore
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var (
		httpServerAddr          string
		geoMMDB, geoParquet     string
		proxyMMDB, proxyParquet string
		pprofAddr               string
		debug                   bool
	)

	flag.StringVar(&httpServerAddr, "addr", "0.0.0.0:8080", "http server address")
	flag.StringVar(&geoMMDB, "geo-mmdb", "", "path to geo mmdb file")
	flag.StringVar(&geoParquet, "geo-parquet", "", "path to geo parquet file")
	flag.StringVar(&proxyMMDB, "proxy-mmdb", "", "path to proxy mmdb file")
	flag.StringVar(&proxyParquet, "proxy-parquet", "", "path to proxy parquet file")
	flag.StringVar(&pprofAddr, "pprof", "", "pprof handler address")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.Parse()

	logger, _ := Logger(debug, 0)
	defer LogPanic(logger)

	defer Pprof(pprofAddr, logger)()

	geo := UnwrapOrExit(NewDataset[GeoRecord, GeoRow](geoMMDB, geoParquet, log.WithPrefix(logger, "geo")))
	defer closeCloser(geo, "geo dataset", logger)

	proxy := UnwrapOrExit(NewDataset[ProxyInfo, ProxyInfo](proxyMMDB, proxyParquet, log.WithPrefix(logger, "proxy")))
	defer closeCloser(proxy, "proxy dataset", logger)

	httpServer := NewHTTPServer(httpServerAddr, logger, geo, proxy)
	go Serve(ctx, httpServer)

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
