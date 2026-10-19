package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	var err error
	switch os.Args[1] {
	case "export":
		err = runExport(ctx, os.Args[2:])
	case "delete":
		err = runDelete(ctx, os.Args[2:])
	default:
		usage()
	}
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "usage:\n  %s export -from DATE -to DATE -o FILE <dir>\n", name)
	fmt.Fprintf(os.Stderr, "  %s delete -from DATE -to DATE [-net CIDR]... [-asn N]... [-dry-run] <dir>\n", name)
	os.Exit(2)
}

func parseDay(s string) (time.Time, error) {
	return time.ParseInLocation(time.DateOnly, s, time.UTC)
}

func parseRange(from, to string) (conntrack.TimeRange, error) {
	start, err := parseDay(from)
	if err != nil {
		return conntrack.TimeRange{}, fmt.Errorf("invalid -from: %w", err)
	}
	end, err := parseDay(to)
	if err != nil {
		return conntrack.TimeRange{}, fmt.Errorf("invalid -to: %w", err)
	}
	tr := conntrack.TimeRange{
		Start: conntrack.Timestamp(start.Unix()),
		End:   conntrack.Timestamp(end.AddDate(0, 0, 1).Unix() - 1),
	}
	if !tr.Valid() {
		return conntrack.TimeRange{}, fmt.Errorf("-from %s is after -to %s", from, to)
	}
	return tr, nil
}

func openStore(dir string) conntrack.Store {
	return conntrack.NewFileStore(dir, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
}
