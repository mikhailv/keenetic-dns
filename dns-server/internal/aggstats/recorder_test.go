package aggstats

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testKey struct {
	Name  string `tsv:"name"`
	QType string `tsv:"qtype"`
}

type (
	testChunk = Chunk[testKey, Counter]
	testEntry = Entry[testKey, Counter]
)

type memStore struct {
	mu     sync.Mutex
	chunks map[TimeRange]testChunk
	saves  int
	closed bool
	err    error
}

func newMemStore() *memStore { return &memStore{chunks: map[TimeRange]testChunk{}} }

func (s *memStore) Init(context.Context) error { return nil }

func (s *memStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *memStore) Save(_ context.Context, chunk testChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.err != nil {
		return s.err
	}
	s.chunks[chunk.TimeRange] = chunk
	return nil
}

func (s *memStore) Load(context.Context, TimeRange) iter.Seq2[testChunk, error] {
	return func(yield func(testChunk, error) bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, chunk := range s.chunks {
			if !yield(chunk, nil) {
				return
			}
		}
	}
}

func (s *memStore) chunk(t *testing.T, tr TimeRange) testChunk {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	chunk, ok := s.chunks[tr]
	require.True(t, ok, "no chunk saved for %s", tr)
	return chunk
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var hourStart = time.Unix(1754784000, 0).UTC()

func firstHour() TimeRange {
	return TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599}
}

func newTestRecorder(t *testing.T, cfg Config) (*Recorder[testKey, Counter], *memStore, *clock) {
	t.Helper()
	store := newMemStore()
	c := &clock{now: hourStart}
	r := NewRecorder[testKey, Counter](store, cfg, Counter.Clone, slog.New(slog.DiscardHandler))
	r.now = c.Now
	return r, store, c
}

func record(r *Recorder[testKey, Counter], name string) {
	r.Record(testKey{Name: name, QType: "A"}, func(v *Counter, offset uint16) { v.Add(offset) })
}

func findEntry(entries []testEntry, name string) (testEntry, bool) {
	i := slices.IndexFunc(entries, func(e testEntry) bool { return e.Key.Name == name })
	if i < 0 {
		return testEntry{}, false
	}
	return entries[i], true
}

func TestRecorder_AggregatesByKey(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{})

	record(r, "a")
	c.advance(30 * time.Second)
	record(r, "a")
	r.Record(testKey{Name: "a", QType: "AAAA"}, func(v *Counter, offset uint16) { v.Add(offset) })

	require.NoError(t, r.Flush(t.Context()))
	chunk := store.chunk(t, firstHour())
	require.Len(t, chunk.Entries, 2, "every key field is part of the key")
	a, ok := findEntry(chunk.Entries, "a")
	require.True(t, ok)
	assert.Positive(t, a.Value.Count)
}

func TestRecorder_UpdateDefinesAggregation(t *testing.T) {
	type bytesKey struct {
		Name string `tsv:"name"`
	}
	type bytesValue struct {
		Bytes uint64 `tsv:"bytes"`
		Max   uint64 `tsv:"max"`
	}
	store := &recordingStore[bytesKey, bytesValue]{}
	r := NewRecorder[bytesKey, bytesValue](store, Config{}, nil, slog.New(slog.DiscardHandler))
	r.now = func() time.Time { return hourStart }

	for _, n := range []uint64{10, 30, 20} {
		r.Record(bytesKey{Name: "x"}, func(v *bytesValue, _ uint16) {
			v.Bytes += n
			v.Max = max(v.Max, n)
		})
	}
	require.NoError(t, r.Flush(t.Context()))
	require.Len(t, store.saved, 1)
	assert.Equal(t, []Entry[bytesKey, bytesValue]{{Key: bytesKey{Name: "x"}, Value: bytesValue{Bytes: 60, Max: 30}}},
		store.saved[0].Entries)
}

type recordingStore[K comparable, V any] struct {
	Store[K, V]
	saved []Chunk[K, V]
}

func (s *recordingStore[K, V]) Save(_ context.Context, chunk Chunk[K, V]) error {
	s.saved = append(s.saved, chunk)
	return nil
}

func TestRecorder_RecordsOffsets(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{})

	record(r, "a")
	c.advance(12 * time.Second)
	record(r, "a")
	c.advance(33 * time.Second)
	record(r, "a")

	require.NoError(t, r.Flush(t.Context()))
	entry := store.chunk(t, firstHour()).Entries[0]
	assert.Equal(t, uint32(3), entry.Value.Count)
	assert.Equal(t, Offsets{0, 12, 45}, entry.Value.Ts)
	assert.True(t, slices.IsSorted(entry.Value.Ts), "offsets must be ascending")
}

func TestRecorder_RotatesOnChunkBoundary(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{ChunkDuration: time.Hour})

	record(r, "first")
	c.advance(time.Hour)
	record(r, "second")
	require.NoError(t, r.Flush(t.Context()))

	firstRange := firstHour()
	secondRange := TimeRange{Start: firstRange.Start + 3600, End: firstRange.End + 3600}

	first := store.chunk(t, firstRange)
	require.Len(t, first.Entries, 1)
	assert.Equal(t, "first", first.Entries[0].Key.Name)

	second := store.chunk(t, secondRange)
	require.Len(t, second.Entries, 1)
	assert.Equal(t, "second", second.Entries[0].Key.Name)
	assert.Equal(t, Offsets{0}, second.Entries[0].Value.Ts, "offsets restart at the new chunk")
}

func TestRecorder_KeepsEveryKey(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})

	const keys = 60_000
	for i := range keys {
		record(r, strconv.Itoa(i))
	}
	require.NoError(t, r.Flush(t.Context()))
	assert.Len(t, store.chunk(t, firstHour()).Entries, keys, "nothing is dropped, however many keys a chunk has")
}

func TestRecorder_OffsetCapKeepsCountAuthoritative(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})

	const hits = maxOffsets + 500
	for range hits {
		record(r, "loop")
	}

	require.NoError(t, r.Flush(t.Context()))
	entry := store.chunk(t, firstHour()).Entries[0]
	assert.Equal(t, uint32(hits), entry.Value.Count, "count must not be capped")
	assert.Len(t, entry.Value.Ts, maxOffsets, "offsets become a sample")
}

func TestRecorder_FlushIsIdempotentWhenIdle(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})

	require.NoError(t, r.Flush(t.Context()))
	assert.Zero(t, store.saves, "nothing recorded means nothing to write")

	record(r, "a")
	require.NoError(t, r.Flush(t.Context()))
	assert.Equal(t, 1, store.saves)
}

func TestRecorder_RewritesCurrentChunkOnEachFlush(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{})

	record(r, "a")
	require.NoError(t, r.Flush(t.Context()))
	assert.Equal(t, uint32(1), store.chunk(t, firstHour()).Entries[0].Value.Count)

	c.advance(time.Minute)
	record(r, "a")
	require.NoError(t, r.Flush(t.Context()))
	assert.Equal(t, uint32(2), store.chunk(t, firstHour()).Entries[0].Value.Count)
	assert.Equal(t, 2, store.saves)
}

func TestRecorder_FlushedChunkIsNotChangedByLaterRecords(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{})

	record(r, "a")
	require.NoError(t, r.Flush(t.Context()))
	saved := store.chunk(t, firstHour())

	c.advance(time.Second)
	record(r, "a")
	assert.Equal(t, Offsets{0}, saved.Entries[0].Value.Ts, "a saved chunk holds its own copy of the offsets")
}

func TestRecorder_CloseFlushesWithCancelledContext(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})
	record(r, "a")

	require.NoError(t, r.Close())
	assert.Equal(t, 1, store.saves, "shutdown must not drop unflushed history")
}

func TestRecorder_CloseLeavesStoreOpen(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})
	record(r, "a")

	require.NoError(t, r.Close())

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, 1, store.saves, "closing must still flush")
	assert.False(t, store.closed, "closing the store is the caller's business")
}

func TestRecorder_ConcurrentRecording(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for range 200 {
				record(r, "shared")
				record(r, "other"+string(rune('a'+i)))
			}
		})
	}
	wg.Wait()
	require.NoError(t, r.Flush(t.Context()))

	chunk := store.chunk(t, firstHour())
	shared, ok := findEntry(chunk.Entries, "shared")
	require.True(t, ok)
	assert.Equal(t, uint32(8*200), shared.Value.Count)
	assert.Len(t, chunk.Entries, 9, "one shared key plus one per goroutine")
}

func TestRecorderConfig_Defaults(t *testing.T) {
	var cfg Config
	cfg.SetDefaults()
	assert.Equal(t, DefaultChunkDuration, cfg.ChunkDuration)
	assert.Equal(t, DefaultFlushInterval, cfg.FlushInterval)

	cfg = Config{ChunkDuration: 48 * time.Hour}
	cfg.SetDefaults()
	assert.Equal(t, time.Duration(maxChunkSeconds)*time.Second, cfg.ChunkDuration)
}

func TestRecorderConfig_Validate(t *testing.T) {
	cfg := Config{Enabled: true, DataDir: "stats", FlushInterval: 2 * time.Hour, ChunkDuration: time.Hour}
	require.ErrorContains(t, cfg.Validate("blockstats"), "blockstats: flush_interval")

	cfg = Config{Enabled: true, ChunkDuration: time.Hour, FlushInterval: time.Minute}
	require.ErrorContains(t, cfg.Validate("blockstats"), "blockstats: data_dir")

	cfg.Enabled = false
	assert.NoError(t, cfg.Validate("blockstats"), "a disabled section is not checked")
}

func TestRecorder_FlushKeepsChunksWhoseSaveFailed(t *testing.T) {
	r, store, clk := newTestRecorder(t, Config{})

	store.err = errors.New("disk full")
	record(r, "a")
	clk.advance(2 * time.Hour)
	record(r, "b")

	require.Error(t, r.Flush(t.Context()))

	store.err = nil
	require.NoError(t, r.Flush(t.Context()))

	var names []string
	for _, c := range store.chunks {
		for _, e := range c.Entries {
			names = append(names, e.Key.Name)
		}
	}
	assert.Contains(t, names, "a", "a chunk whose save failed must be retried, not dropped")
}
