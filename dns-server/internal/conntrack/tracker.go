package conntrack

import (
	"bytes"
	"cmp"
	"context"
	"iter"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

// TrackerConfig holds configuration for the conntrack tracker.
type TrackerConfig struct {
	PollInterval   time.Duration
	BucketInterval time.Duration
	ChunkInterval  time.Duration
	SaveInterval   time.Duration
	CacheDuration  time.Duration
}

// Tracker polls conntrack periodically, computes deltas, aggregates into time buckets,
// groups buckets into chunks, and persists completed chunks via Store.
type Tracker struct {
	cfg    TrackerConfig
	logger *slog.Logger
	agent  agentclient.NetworkServiceClient
	store  Store
	stream stream.Stream[Bucket]

	mu           sync.RWMutex
	prevSnapshot map[snapshotKey]snapshotEntry
	chunk        Chunk
	bucketRange  TimeRange
	bucketData   map[ConnKey]*bucketAccumulator
	cached       []Chunk
}

func NewTracker(
	cfg TrackerConfig,
	logger *slog.Logger,
	agent agentclient.NetworkServiceClient,
	store Store,
	stream stream.Stream[Bucket],
) *Tracker {
	ensureDurationMinuteBounded(cfg.BucketInterval)
	ensureDurationMinuteBounded(cfg.ChunkInterval)
	now := time.Now()
	return &Tracker{
		cfg:          cfg,
		logger:       logger,
		agent:        agent,
		store:        store,
		stream:       stream,
		prevSnapshot: map[snapshotKey]snapshotEntry{},
		bucketRange:  makeRange(now, cfg.BucketInterval),
		bucketData:   map[ConnKey]*bucketAccumulator{},
		chunk: Chunk{
			TimeRange:      makeRange(now, cfg.ChunkInterval),
			BucketDuration: uint(cfg.BucketInterval.Seconds()),
			Buckets:        map[Timestamp]Bucket{},
		},
	}
}

// Start launches the polling loop. Blocks until ctx is cancelled.
func (s *Tracker) Start(ctx context.Context) {
	s.loadCurrent(ctx)
	s.loadCached(ctx)

	pollTicker := time.NewTicker(s.cfg.PollInterval)
	defer pollTicker.Stop()

	saveTicker := time.NewTicker(s.cfg.SaveInterval)
	defer saveTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.flush(context.WithoutCancel(ctx))
			return
		case now := <-pollTicker.C:
			s.broadcastChangedEntries(s.poll(ctx, now))
		case <-saveTicker.C:
			s.mu.RLock()
			s.saveChunk(ctx)
			s.mu.RUnlock()
		}
	}
}

func (s *Tracker) IterateChunks(ctx context.Context, tr TimeRange) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		if !tr.Valid() {
			return
		}

		s.mu.RLock()
		var loaded []Chunk
		if s.chunk.TimeRange.Intersects(tr) {
			loaded = append(loaded, s.snapshotCurrentChunk())
			tr.End = s.chunk.TimeRange.Start - 1
		}
		for i := len(s.cached) - 1; i >= 0; i-- {
			if !tr.Valid() {
				break
			}
			it := &s.cached[i]
			if it.TimeRange.Intersects(tr) {
				loaded = append(loaded, it.Clone())
				tr.End = it.TimeRange.Start - 1
			}
		}
		s.mu.RUnlock()

		if tr.Valid() {
			for chunk, err := range s.store.Load(ctx, tr) {
				if !yield(chunk, err) {
					return
				}
			}
		}

		slices.Reverse(loaded)
		for _, chunk := range loaded {
			if !yield(chunk, nil) {
				return
			}
		}
	}
}

func (s *Tracker) broadcastChangedEntries(changed util.Set[ConnKey]) {
	if len(changed) == 0 {
		return
	}

	s.mu.RLock()
	bucket := Bucket{
		TimeRange: s.bucketRange,
		Entries:   make([]BucketEntry, 0, len(changed)),
	}
	for ck := range changed {
		if acc := s.bucketData[ck]; acc != nil {
			bucket.Entries = append(bucket.Entries, acc.toBucketEntry(ck))
		}
	}
	s.mu.RUnlock()

	s.stream.Append(bucket)
}

func (s *Tracker) snapshotCurrentChunk() Chunk {
	chunk := s.chunk.Clone()
	bucket := s.snapshotCurrentBucket()
	chunk.Buckets[bucket.TimeRange.Start] = bucket
	return chunk
}

func (s *Tracker) snapshotCurrentBucket() Bucket {
	entries := make([]BucketEntry, 0, len(s.bucketData))
	for key, acc := range s.bucketData {
		entries = append(entries, acc.toBucketEntry(key))
	}
	slices.SortFunc(entries, func(a, b BucketEntry) int {
		return cmp.Or(
			bytes.Compare(a.SrcIP[:], b.SrcIP[:]),
			cmp.Compare(a.Protocol, b.Protocol),
			bytes.Compare(a.DstIP[:], b.DstIP[:]),
			cmp.Compare(a.DstPort, b.DstPort),
			bytes.Compare(a.MAC[:], b.MAC[:]),
		)
	})
	return Bucket{
		TimeRange: s.bucketRange,
		Entries:   entries,
	}
}

func (s *Tracker) sealBucket() {
	if len(s.bucketData) == 0 {
		return
	}
	bucket := s.snapshotCurrentBucket()
	s.chunk.Buckets[bucket.TimeRange.Start] = bucket
	clear(s.bucketData)
}

func (s *Tracker) poll(ctx context.Context, now time.Time) util.Set[ConnKey] {
	entries, err := s.agent.ListConntrack(ctx)
	if err != nil {
		s.logger.Error("failed to poll conntrack", "err", err)
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nowTs := Timestamp(now.Unix())

	if nowTs > s.bucketRange.End {
		s.sealBucket()
		s.bucketRange = makeRange(now, s.cfg.BucketInterval)
		if nowTs > s.chunk.TimeRange.End {
			s.saveChunk(ctx)
			s.cached = append(s.cached, s.chunk.Clone())
			s.evictOldCachedChunks()
			s.chunk.TimeRange = makeRange(now, s.cfg.ChunkInterval)
			clear(s.chunk.Buckets)
		}
	}

	changedKeys := make(util.Set[ConnKey], len(entries))

	// Build new snapshot and compute deltas.
	newSnapshot := make(map[snapshotKey]snapshotEntry, len(entries))
	for _, e := range entries {
		sk := snapshotKey{
			Protocol: ParseProtocol(e.Protocol),
			SrcIP:    ParseIP(e.SrcIp),
			SrcPort:  util.Deref(e.SrcPort),
			DstIP:    ParseIP(e.DstIp),
			DstPort:  util.Deref(e.DstPort),
			MAC:      ParseMAC(util.Deref(e.Mac)),
		}
		se := snapshotEntry{
			BytesOrig:    e.BytesOrig,
			BytesReply:   e.BytesReply,
			PacketsOrig:  uint32(min(e.PacketsOrig, math.MaxUint32)),
			PacketsReply: uint32(min(e.PacketsReply, math.MaxUint32)),
		}
		newSnapshot[sk] = se

		ck := ConnKey{
			Protocol: sk.Protocol,
			SrcIP:    sk.SrcIP,
			DstIP:    sk.DstIP,
			DstPort:  sk.DstPort,
			MAC:      sk.MAC,
		}

		var delta snapshotEntry
		if prev, ok := s.prevSnapshot[sk]; ok {
			delta = snapshotEntry{
				BytesOrig:    max(0, se.BytesOrig-prev.BytesOrig),
				BytesReply:   max(0, se.BytesReply-prev.BytesReply),
				PacketsOrig:  saturatingSub(se.PacketsOrig, prev.PacketsOrig),
				PacketsReply: saturatingSub(se.PacketsReply, prev.PacketsReply),
			}
		} else {
			// New connection — absolute counters as first delta.
			delta = se
		}

		acc := s.getOrCreateAccumulator(ck)
		if delta == (snapshotEntry{}) {
			// Still register the src_port for connection counting.
			acc.srcPorts.Add(sk.SrcPort)
		} else {
			acc.addDelta(sk.SrcPort, delta)
			changedKeys.Add(ck)
		}
	}

	s.prevSnapshot = newSnapshot
	return changedKeys
}

func (s *Tracker) getOrCreateAccumulator(ck ConnKey) *bucketAccumulator {
	acc, ok := s.bucketData[ck]
	if !ok {
		acc = &bucketAccumulator{srcPorts: util.Set[uint16]{}}
		s.bucketData[ck] = acc
	}
	return acc
}

func (s *Tracker) saveChunk(ctx context.Context) {
	if err := s.store.Save(ctx, s.chunk); err != nil {
		s.logger.Error("failed to save conntrack chunk", "err", err)
	} else {
		s.logger.Info("saved conntrack chunk", "buckets", len(s.chunk.Buckets), "time_range", s.chunk.TimeRange)
	}
}

func (s *Tracker) evictOldCachedChunks() {
	if s.cfg.CacheDuration <= 0 {
		return
	}
	cutoff := Timestamp(time.Now().Add(-s.cfg.CacheDuration).Unix())
	for len(s.cached) > 0 && s.cached[0].TimeRange.End < cutoff {
		copy(s.cached, s.cached[1:])
		s.cached[len(s.cached)-1] = Chunk{}
		s.cached = s.cached[:len(s.cached)-1]
	}
}

func (s *Tracker) flush(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealBucket()
	s.saveChunk(ctx)
}

func (s *Tracker) loadCurrent(ctx context.Context) {
	for chunk, err := range s.store.Load(ctx, s.chunk.TimeRange) {
		switch {
		case err != nil:
			s.logger.Error("failed to load current chunk", "err", err)
		case chunk.TimeRange != s.chunk.TimeRange:
			s.logger.Info("chunk time range incompatible", "time_range", chunk.TimeRange)
		case chunk.BucketDuration != s.chunk.BucketDuration:
			s.logger.Info("chunk bucket duration incompatible", "bucket_duration", chunk.BucketDuration)
		default:
			s.mu.Lock()
			s.chunk = chunk
			s.mu.Unlock()
			s.logger.Info("current chunk loaded", "buckets", len(chunk.Buckets))
		}
		return
	}
	s.logger.Info("no chunk loaded for current time range")
}

func (s *Tracker) loadCached(ctx context.Context) {
	if s.cfg.CacheDuration <= 0 {
		return
	}
	tr := TimeRange{
		Start: Timestamp(time.Now().Add(-s.cfg.CacheDuration).Unix()),
		End:   s.chunk.TimeRange.Start - 1,
	}
	if !tr.Valid() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for chunk, err := range s.store.Load(ctx, tr) {
		if err != nil {
			s.logger.Error("failed to load cached chunk", "err", err)
			continue
		}
		s.logger.Info("loaded cached conntrack chunk", "buckets", len(chunk.Buckets), "time_range", chunk.TimeRange)
		s.cached = append(s.cached, chunk)
	}
	s.logger.Info("loaded cached conntrack chunks", "count", len(s.cached))
}

func saturatingSub(a, b uint32) uint32 {
	if a < b {
		return 0
	}
	return a - b
}

func ensureDurationMinuteBounded(d time.Duration) {
	if d.Truncate(time.Minute) != d {
		panic("duration must be minute bounded")
	}
}
