package conntrack

import (
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

const keepTrackMissCount = 10

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
	stream *stream.Buffered[Bucket]

	mu            sync.RWMutex
	prevSnapshot  map[snapshotKey]snapshotEntry
	bucketRange   TimeRange
	bucketEntries map[ConnKey]*bucketEntryAccumulator
	chunkRange    TimeRange
	chunkBuckets  map[Timestamp]Bucket
	cached        []Chunk
}

// NewTracker creates a new Tracker with the given configuration.
func NewTracker(
	cfg TrackerConfig,
	logger *slog.Logger,
	agent agentclient.NetworkServiceClient,
	store Store,
	stream *stream.Buffered[Bucket],
) *Tracker {
	ensureDurationMinuteBounded(cfg.BucketInterval)
	ensureDurationMinuteBounded(cfg.ChunkInterval)
	now := time.Now()
	return &Tracker{
		cfg:           cfg,
		logger:        logger,
		agent:         agent,
		store:         store,
		stream:        stream,
		prevSnapshot:  map[snapshotKey]snapshotEntry{},
		bucketRange:   makeRange(now, cfg.BucketInterval),
		bucketEntries: map[ConnKey]*bucketEntryAccumulator{},
		chunkRange:    makeRange(now, cfg.ChunkInterval),
		chunkBuckets:  map[Timestamp]Bucket{},
	}
}

// Stream returns the buffered stream of bucket updates for real-time subscribers.
func (s *Tracker) Stream() *stream.Buffered[Bucket] {
	return s.stream
}

// BucketDuration returns the native bucket size in seconds used by this tracker.
func (s *Tracker) BucketDuration() uint {
	return uint(s.cfg.BucketInterval.Seconds())
}

// Start launches the polling loop.
func (s *Tracker) Start(ctx context.Context) util.Waiter {
	waiter, stop := util.NewWaiter()
	go s.start(ctx, stop)
	return waiter
}

func (s *Tracker) start(ctx context.Context, onStop func()) {
	defer onStop()
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
			chunk := s.snapshotCurrentChunk()
			s.mu.RUnlock()
			s.saveChunk(ctx, chunk)
		}
	}
}

// IterateChunks returns an iterator over chunks that intersect the given time range,
// combining in-memory cached chunks with persisted chunks from the store.
// Chunks are yielded in chronological order.
func (s *Tracker) IterateChunks(ctx context.Context, tr TimeRange) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		if !tr.Valid() {
			return
		}

		s.mu.RLock()
		var loaded []Chunk
		if s.chunkRange.Intersects(tr) {
			chunk := s.snapshotCurrentChunk()
			loaded = append(loaded, chunk)
			tr.End = s.chunkRange.Start - 1
		}
		for i := len(s.cached) - 1; i >= 0; i-- {
			if !tr.Valid() {
				break
			}
			it := s.cached[i]
			if it.TimeRange.Intersects(tr) {
				loaded = append(loaded, it)
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
		if acc := s.bucketEntries[ck]; acc != nil {
			bucket.Entries = append(bucket.Entries, acc.toBucketEntry(ck))
		}
	}
	s.mu.RUnlock()

	s.stream.Append(bucket)
}

func (s *Tracker) snapshotCurrentChunk() Chunk {
	buckets := make([]Bucket, 0, len(s.chunkBuckets)+1)
	for _, bucket := range s.chunkBuckets {
		buckets = append(buckets, bucket)
	}
	if len(s.bucketEntries) > 0 {
		buckets = append(buckets, s.snapshotCurrentBucket())
	}
	SortBuckets(buckets)
	return Chunk{
		TimeRange:      s.chunkRange,
		BucketDuration: s.BucketDuration(),
		Buckets:        buckets,
	}
}

func (s *Tracker) snapshotCurrentBucket() Bucket {
	entries := make([]BucketEntry, 0, len(s.bucketEntries))
	for key, acc := range s.bucketEntries {
		entries = append(entries, acc.toBucketEntry(key))
	}
	SortBucketEntries(entries)
	return Bucket{
		TimeRange: s.bucketRange,
		Entries:   entries,
	}
}

func (s *Tracker) sealBucket() {
	if len(s.bucketEntries) == 0 {
		return
	}
	bucket := s.snapshotCurrentBucket()
	s.chunkBuckets[bucket.TimeRange.Start] = bucket
	clear(s.bucketEntries)
}

func (s *Tracker) poll(ctx context.Context, now time.Time) util.Set[ConnKey] { //nolint:funlen,gocognit // ignore
	entries, err := s.agent.ListConntrack(ctx)
	if err != nil {
		s.logger.Error("failed to poll conntrack", "err", err)
		return nil
	}

	var chunkToSave Chunk

	s.mu.Lock()
	nowTs := Timestamp(now.Unix())
	if nowTs > s.bucketRange.End {
		s.sealBucket()
		s.bucketRange = makeRange(now, s.cfg.BucketInterval)
		if nowTs > s.chunkRange.End {
			chunkToSave = s.snapshotCurrentChunk()
			s.cached = append(s.cached, chunkToSave)
			s.evictOldCachedChunks()
			s.chunkRange = makeRange(now, s.cfg.ChunkInterval)
			clear(s.chunkBuckets)
		}
	}
	s.mu.Unlock()

	// save new chunk without holding lock
	s.saveChunk(ctx, chunkToSave)

	s.mu.Lock()
	defer s.mu.Unlock()

	firstPoll := len(s.prevSnapshot) == 0
	changedKeys := make(util.Set[ConnKey], len(entries))

	// Build new snapshot and compute deltas.
	newSnapshot := make(map[snapshotKey]snapshotEntry, len(entries))
	for _, e := range entries {
		ck := ConnKey{
			Protocol: parseProtocol(e.Protocol),
			SrcIP:    parseIP(e.SrcIp),
			DstIP:    parseIP(e.DstIp),
			DstPort:  util.Deref(e.DstPort),
		}
		cs := ConnStat{
			BytesOrig:    e.BytesOrig,
			BytesReply:   e.BytesReply,
			PacketsOrig:  uint32(min(e.PacketsOrig, math.MaxUint32)),
			PacketsReply: uint32(min(e.PacketsReply, math.MaxUint32)),
		}
		sk := snapshotKey{
			ConnKey: ck,
			SrcPort: util.Deref(e.SrcPort),
		}
		se := snapshotEntry{
			ConnStat: cs,
			State:    util.Deref(e.State),
		}
		newSnapshot[sk] = se

		if firstPoll {
			continue
		}

		prev, hasPrev := s.prevSnapshot[sk]

		var delta ConnStat
		switch {
		case !hasPrev && cs.IsLikelyNew():
			delta = cs
		case hasPrev && (prev.MissCount == 0 || cs.IsNextFor(prev.ConnStat)):
			if prev.MissCount > 0 {
				s.logger.Debug("restored entry", "", sk, "", se, "miss_count", prev.MissCount)
			}
			delta = ConnStat{
				BytesOrig:    max(0, cs.BytesOrig-prev.BytesOrig),
				BytesReply:   max(0, cs.BytesReply-prev.BytesReply),
				PacketsOrig:  saturatingSub(cs.PacketsOrig, prev.PacketsOrig),
				PacketsReply: saturatingSub(cs.PacketsReply, prev.PacketsReply),
			}
		default:
			if hasPrev {
				s.logger.Debug("replace entry", "", sk, "", se)
			} else {
				s.logger.Debug("new entry", "", sk, "", se)
			}
		}

		acc := s.getOrCreateAccumulator(ck)
		acc.SrcPorts.Add(sk.SrcPort)
		acc.Add(delta)
		if !delta.IsZero() {
			changedKeys.Add(ck)
		}
	}

	// keep tracking for missing entries
	for sk, se := range s.prevSnapshot {
		if se.State == "TIME_WAIT" || se.State == "CLOSE" {
			continue
		}
		if _, ok := newSnapshot[sk]; !ok {
			se.MissCount++
			if se.MissCount <= keepTrackMissCount {
				newSnapshot[sk] = se
			} else {
				s.logger.Debug("expired entry", "", sk, "", se)
			}
		}
	}

	s.prevSnapshot = newSnapshot
	return changedKeys
}

func (s *Tracker) getOrCreateAccumulator(ck ConnKey) *bucketEntryAccumulator {
	acc, ok := s.bucketEntries[ck]
	if !ok {
		acc = &bucketEntryAccumulator{}
		s.bucketEntries[ck] = acc
	}
	return acc
}

func (s *Tracker) saveChunk(ctx context.Context, chunk Chunk) {
	if !chunk.IsValid() {
		return
	}
	if err := s.store.Save(ctx, chunk); err != nil {
		s.logger.Error("failed to save chunk", "err", err)
	} else {
		s.logger.Info("chunk saved", "buckets", len(chunk.Buckets), "time_range", chunk.TimeRange)
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
	s.saveChunk(ctx, s.snapshotCurrentChunk())
}

func (s *Tracker) loadCurrent(ctx context.Context) {
	for chunk, err := range s.store.Load(ctx, s.chunkRange) {
		switch {
		case err != nil:
			s.logger.Error("failed to load current chunk", "err", err)
		case chunk.TimeRange != s.chunkRange:
			s.logger.Info("chunk time range incompatible", "time_range", chunk.TimeRange, "expected_time_range", s.chunkRange)
		case chunk.BucketDuration != s.BucketDuration():
			s.logger.Info("chunk bucket duration incompatible", "bucket_duration", chunk.BucketDuration, "expected_bucket_duration", s.BucketDuration())
		default:
			s.mu.Lock()
			for _, bucket := range chunk.Buckets {
				if bucket.TimeRange == s.bucketRange {
					for _, entry := range bucket.Entries {
						s.bucketEntries[entry.ConnKey] = &bucketEntryAccumulator{
							ConnStat: entry.ConnStat,
							SrcPorts: util.NewSet(entry.SrcPorts...),
						}
					}
				} else {
					s.chunkBuckets[bucket.TimeRange.Start] = bucket
				}
			}
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
		End:   s.chunkRange.Start - 1,
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
		s.logger.Info("loaded cached chunk", "buckets", len(chunk.Buckets), "time_range", chunk.TimeRange)
		s.cached = append(s.cached, chunk)
	}
	s.logger.Info("loaded cached chunks", "count", len(s.cached))
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
