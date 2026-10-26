package domainstats

import (
	"context"
	"log/slog"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/aggstats"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type Key struct {
	ClientIP types.IPv4 `tsv:"client_ip"`
	Domain   string     `tsv:"domain"`
	QType    string     `tsv:"qtype"`
	Label    string     `tsv:"label,optional"`
}

type (
	Chunk = aggstats.Chunk[Key, aggstats.Counter]
	Entry = aggstats.Entry[Key, aggstats.Counter]
	Store = aggstats.Store[Key, aggstats.Counter]
)

func NewFileStore(dir string, logger *slog.Logger) Store {
	return aggstats.NewFileStore[Key, aggstats.Counter](dir, normalizeKey, logger)
}

func normalizeKey(k *Key) {
	k.Domain = util.FQDN(k.Domain)
}

type Recorder struct {
	rec *aggstats.Recorder[Key, aggstats.Counter]
}

func NewRecorder(store Store, cfg aggstats.Config, logger *slog.Logger) *Recorder {
	return &Recorder{rec: aggstats.NewRecorder(store, cfg, aggstats.Counter.Clone, logger)}
}

func (r *Recorder) Record(clientIP types.IPv4, domain, qtype, label string) {
	key := Key{ClientIP: clientIP, Domain: util.FQDN(domain), QType: qtype, Label: label}
	r.rec.Record(key, func(v *aggstats.Counter, offset uint16) {
		v.Add(offset)
	})
}

func (r *Recorder) Start(ctx context.Context) util.Waiter {
	return r.rec.Start(ctx)
}

func (r *Recorder) Flush(ctx context.Context) error {
	return r.rec.Flush(ctx)
}

func (r *Recorder) Close() error {
	return r.rec.Close()
}
