package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/log"
	. "github.com/mikhailv/keenetic-dns/internal/setup" //nolint:staticcheck //ignore
	"github.com/mikhailv/keenetic-dns/internal/util"
	. "github.com/mikhailv/keenetic-dns/ipinfo/internal" //nolint:staticcheck //ignore
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var httpServerAddr string
	var dbPath string
	var pprofAddr string
	var debug bool

	flag.StringVar(&httpServerAddr, "addr", "0.0.0.0:8080", "http server address")
	flag.StringVar(&dbPath, "db", "geolite2.mmdb", "path to MMDB or Parquet file")
	flag.StringVar(&pprofAddr, "pprof", "", "pprof handler address")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.Parse()

	logger, logFlush := Logger(debug, 300)
	defer logFlush()
	defer util.RunPeriodically(ctx.Done(), 10*time.Second, logFlush).Wait()

	defer Pprof(pprofAddr, logger)()

	var resolver Resolver
	if strings.HasSuffix(dbPath, ".parquet") {
		resolver = Unwrap(NewParquetResolver(dbPath, log.WithPrefix(logger, "parquet")))
	} else {
		resolver = Unwrap(NewMMDBResolver(dbPath))
	}
	defer closeCloser(resolver, "resolver", logger)

	httpServer := NewHTTPServer(httpServerAddr, logger, resolver)
	go Serve(ctx, httpServer)

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
