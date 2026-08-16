package blockstats

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const OverflowDomain = "*overflow*"

type Recorder struct {
	store  Store
	cfg    Config
	logger *slog.Logger
	now    func() time.Time

	mu           sync.Mutex
	current      TimeRange
	entries      map[Key]entryState
	overflow     uint32
	dirty        bool
	pending      []Chunk
	cappedLogged bool
}

type entryState struct {
	count   uint32
	offsets Offsets
}

func NewRecorder(store Store, cfg Config, logger *slog.Logger) *Recorder {
	cfg.SetDefaults()
	return &Recorder{
		store:   store,
		cfg:     cfg,
		logger:  logger,
		now:     time.Now,
		entries: map[Key]entryState{},
	}
}

func (r *Recorder) Start(ctx context.Context) util.Waiter {
	return util.RunPeriodically(ctx.Done(), r.cfg.FlushInterval, func() {
		if err := r.Flush(ctx); err != nil && ctx.Err() == nil {
			r.logger.Error("failed to flush blockstats", "err", err)
		}
	})
}

func (r *Recorder) Close() error {
	return r.Flush(context.Background())
}

func (r *Recorder) RecordBlocked(clientIP types.IPv4, domain, qtype, list string) {
	now := r.now()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rotate(now)

	key := Key{ClientIP: clientIP, Domain: domain, QType: qtype, List: list}
	state, ok := r.entries[key]
	if !ok && len(r.entries) >= r.cfg.MaxEntries {
		r.overflow++
		r.dirty = true
		if !r.cappedLogged {
			r.cappedLogged = true
			r.logger.Warn("blockstats entry cap reached, further entries are counted as overflow",
				"max_entries", r.cfg.MaxEntries)
		}
		return
	}

	state.count++
	if len(state.offsets) < maxOffsets {
		state.offsets = append(state.offsets, uint16(now.Unix()-int64(r.current.Start)))
	}
	r.entries[key] = state
	r.dirty = true
}

func (r *Recorder) rotate(now time.Time) {
	if r.current.Valid() && r.current.InRange(Timestamp(now.Unix())) {
		return
	}
	if r.dirty {
		r.pending = append(r.pending, r.chunk())
	}
	start := now.Truncate(r.cfg.ChunkDuration)
	r.current = TimeRange{
		Start: Timestamp(start.Unix()),
		End:   Timestamp(start.Add(r.cfg.ChunkDuration).Unix() - 1),
	}
	r.entries = make(map[Key]entryState, len(r.entries))
	r.overflow = 0
	r.dirty = false
	r.cappedLogged = false
}

func (r *Recorder) chunk() Chunk {
	entries := make([]Entry, 0, len(r.entries)+1)
	for key, state := range r.entries {
		entries = append(entries, Entry{
			ClientIP: key.ClientIP,
			Domain:   key.Domain,
			QType:    key.QType,
			List:     key.List,
			Count:    state.count,
			Ts:       state.offsets.clone(),
		})
	}
	if r.overflow > 0 {
		entries = append(entries, Entry{Domain: OverflowDomain, Count: r.overflow})
	}
	return Chunk{TimeRange: r.current, Entries: entries}
}

func (r *Recorder) Flush(ctx context.Context) error {
	r.mu.Lock()
	pending := r.pending
	r.pending = nil
	var current *Chunk
	if r.dirty {
		chunk := r.chunk()
		current = &chunk
	}
	r.mu.Unlock()

	var errs []error
	var failed []Chunk
	for _, chunk := range pending {
		if err := r.store.Save(ctx, chunk); err != nil {
			r.logger.Error("failed to save blockstats chunk", "range", chunk.TimeRange, "err", err)
			errs = append(errs, err)
			failed = append(failed, chunk)
		}
	}
	if len(failed) > 0 {
		r.mu.Lock()
		r.pending = append(failed, r.pending...)
		r.mu.Unlock()
	}
	if current != nil {
		if err := r.store.Save(ctx, *current); err != nil {
			r.logger.Error("failed to save blockstats chunk", "range", current.TimeRange, "err", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
