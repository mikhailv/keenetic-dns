package internal

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"net"

	"github.com/oschwald/maxminddb-golang"

	"github.com/mikhailv/keenetic-dns/ipinfo/internal/parquetdb"
)

// Dataset is a typed view of an IP geo database. Per-IP lookups go through the mmdb backend (in-memory,
// microsecond-scale) and decode into M; field-level wildcard queries go through the parquet backend and yield P.
//
// M and P may be the same type when one struct fits both schemas, but more often differ: mmdb data is usually nested
// while parquet is flat.
type Dataset[M, P any] struct {
	mmdb    *maxminddb.Reader
	parquet *parquetdb.DB[P]
}

// NewDataset opens an mmdb file and its parquet companion. On error any partially opened resource is released.
func NewDataset[M, P any](mmdbPath, parquetPath string, logger *slog.Logger) (*Dataset[M, P], error) {
	mmdb, err := maxminddb.Open(mmdbPath)
	if err != nil {
		return nil, fmt.Errorf("opening mmdb %q: %w", mmdbPath, err)
	}
	pf, err := parquetdb.Open[P](parquetPath, logger)
	if err != nil {
		_ = mmdb.Close()
		return nil, err
	}
	return &Dataset[M, P]{mmdb: mmdb, parquet: pf}, nil
}

// Lookup decodes the mmdb record at ip into data and returns the CIDR network the record applies to. data is left at
// its zero value and network is empty when no record exists for ip.
func (d *Dataset[M, P]) Lookup(_ context.Context, ip net.IP, data *M) (network string, err error) {
	nw, ok, err := d.mmdb.LookupNetwork(ip, data)
	if err != nil {
		return "", fmt.Errorf("mmdb lookup: %w", err)
	}
	if !ok || nw == nil {
		return "", nil
	}
	return nw.String(), nil
}

// Query streams rows whose `field` column matches the shell-glob query. '*' matches any sequence, '?' matches exactly
// one rune. Matching is case-sensitive and anchored; if query has no glob characters this degrades to an exact-string
// equality. Callers may break early to limit the number of results.
func (d *Dataset[M, P]) Query(ctx context.Context, field, query string) iter.Seq2[P, error] {
	return d.parquet.Query(ctx, parquetdb.Wildcard(field, query))
}

// Close releases both backends. Errors are joined.
func (d *Dataset[M, P]) Close() error {
	return errors.Join(d.mmdb.Close(), d.parquet.Close())
}
