package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mikhailv/keenetic-dns/internal/log"
	. "github.com/mikhailv/keenetic-dns/internal/setup"  //nolint:staticcheck //ignore
	. "github.com/mikhailv/keenetic-dns/ipinfo/internal" //nolint:staticcheck //ignore
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var httpServerAddr string
	var dbFiles dbListFlag
	var pprofAddr string
	var debug bool

	flag.StringVar(&httpServerAddr, "addr", "0.0.0.0:8080", "http server address")
	flag.Var(&dbFiles, "db", "path to MMDB or Parquet file")
	flag.StringVar(&pprofAddr, "pprof", "", "pprof handler address")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.Parse()

	logger, _ := Logger(debug, 0)
	defer LogPanic(logger)

	defer Pprof(pprofAddr, logger)()

	resolvers := make([]Resolver, 0, len(dbFiles))
	for _, df := range dbFiles {
		name, path := df.Name, df.Path
		var resolver Resolver
		if strings.HasSuffix(path, ".parquet") {
			resolver = UnwrapOrExit(NewParquetResolver(name, path, log.WithPrefix(logger, name)))
		} else {
			resolver = UnwrapOrExit(NewMMDBResolver(name, path))
		}
		logger.Info("register resolver", "name", resolver.Name(), "path", path)
		defer closeCloser(resolver, "resolver", logger.With("name", resolver.Name()))
		resolvers = append(resolvers, resolver)
	}

	httpServer := NewHTTPServer(httpServerAddr, logger, NewResolverRegistry(resolvers))
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

type dbFile struct {
	Name string
	Path string
}

var _ flag.Value = (*dbListFlag)(nil)

type dbListFlag []dbFile

func (f *dbListFlag) String() string {
	return fmt.Sprintf("%v", *f)
}

// Set is an implementation of the flag.Value interface.
func (f *dbListFlag) Set(value string) error {
	if value == "" {
		return nil
	}
	before, after, ok := strings.Cut(value, ":")
	if !ok {
		*f = append(*f, dbFile{
			Name: strings.TrimSuffix(filepath.Base(value), filepath.Ext(value)),
			Path: value,
		})
	} else {
		*f = append(*f, dbFile{
			Name: before,
			Path: after,
		})
	}
	return nil
}
