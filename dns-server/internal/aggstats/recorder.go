package aggstats

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

type Recorder[K comparable, V any] struct {
	store  Store[K, V]
	cfg    Config
	clone  func(V) V
	logger *slog.Logger
	now    func() time.Time

	mu      sync.Mutex
	current TimeRange
	entries map[K]*V
	dirty   bool
	pending []Chunk[K, V]
}

func NewRecorder[K comparable, V any](store Store[K, V], cfg Config, clone func(V) V, logger *slog.Logger) *Recorder[K, V] {
	cfg.SetDefaults()
	if clone == nil {
		clone = func(v V) V { return v }
	}
	return &Recorder[K, V]{
		store:   store,
		cfg:     cfg,
		clone:   clone,
		logger:  logger,
		now:     time.Now,
		entries: map[K]*V{},
	}
}

func (r *Recorder[K, V]) Start(ctx context.Context) util.Waiter {
	return util.RunPeriodically(ctx.Done(), r.cfg.FlushInterval, func() {
		if err := r.Flush(ctx); err != nil && ctx.Err() == nil {
			r.logger.Error("failed to flush stats", "err", err)
		}
	})
}

func (r *Recorder[K, V]) Close() error {
	return r.Flush(context.Background())
}

func (r *Recorder[K, V]) Record(key K, update func(v *V, offset uint16)) {
	now := r.now()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rotate(now)

	v, ok := r.entries[key]
	if !ok {
		v = new(V)
		r.entries[key] = v
	}
	update(v, uint16(now.Unix()-int64(r.current.Start)))
	r.dirty = true
}

func (r *Recorder[K, V]) rotate(now time.Time) {
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
	r.entries = make(map[K]*V, len(r.entries))
	r.dirty = false
}

func (r *Recorder[K, V]) chunk() Chunk[K, V] {
	entries := make([]Entry[K, V], 0, len(r.entries))
	for key, v := range r.entries {
		entries = append(entries, Entry[K, V]{Key: key, Value: r.clone(*v)})
	}
	return Chunk[K, V]{TimeRange: r.current, Entries: entries}
}

func (r *Recorder[K, V]) Flush(ctx context.Context) error {
	r.mu.Lock()
	pending := r.pending
	r.pending = nil
	var current *Chunk[K, V]
	if r.dirty {
		chunk := r.chunk()
		current = &chunk
	}
	r.mu.Unlock()

	var errs []error
	var failed []Chunk[K, V]
	for _, chunk := range pending {
		if err := r.store.Save(ctx, chunk); err != nil {
			r.logger.Error("failed to save stats chunk", "range", chunk.TimeRange, "err", err)
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
			r.logger.Error("failed to save stats chunk", "range", current.TimeRange, "err", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
