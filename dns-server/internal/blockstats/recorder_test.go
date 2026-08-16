package blockstats

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type memStore struct {
	mu     sync.Mutex
	chunks map[TimeRange]Chunk
	saves  int
	closed bool
	err    error
}

func newMemStore() *memStore { return &memStore{chunks: map[TimeRange]Chunk{}} }

func (s *memStore) Init(context.Context) error { return nil }

func (s *memStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *memStore) Save(_ context.Context, chunk Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.err != nil {
		return s.err
	}
	s.chunks[chunk.TimeRange] = chunk
	return nil
}

func (s *memStore) Load(context.Context, TimeRange) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, chunk := range s.chunks {
			if !yield(chunk, nil) {
				return
			}
		}
	}
}

func (s *memStore) chunk(t *testing.T, tr TimeRange) Chunk {
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

func newTestRecorder(t *testing.T, cfg Config) (*Recorder, *memStore, *clock) {
	t.Helper()
	store := newMemStore()
	c := &clock{now: hourStart}
	r := NewRecorder(store, cfg, slog.New(slog.DiscardHandler))
	r.now = c.Now
	return r, store, c
}

func findEntry(entries []Entry, domain string) (Entry, bool) {
	i := slices.IndexFunc(entries, func(e Entry) bool { return e.Domain == domain })
	if i < 0 {
		return Entry{}, false
	}
	return entries[i], true
}

func TestRecorder_AggregatesByKey(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{})
	client := types.MustParseIPv4("192.168.1.42")

	r.RecordBlocked(client, "ads.example.com", "A", "hagezi-pro")
	c.advance(30 * time.Second)
	r.RecordBlocked(client, "ads.example.com", "A", "hagezi-pro")
	c.advance(30 * time.Second)
	r.RecordBlocked(client, "ads.example.com", "AAAA", "hagezi-pro")

	require.NoError(t, r.Flush(t.Context()))

	chunk := store.chunk(t, TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599})
	require.Len(t, chunk.Entries, 2, "query type is part of the key")

	a, ok := findEntry(chunk.Entries, "ads.example.com")
	require.True(t, ok)
	assert.Equal(t, client, a.ClientIP)
	assert.Equal(t, "hagezi-pro", a.List)
}

func TestRecorder_RecordsOffsets(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{})
	client := types.MustParseIPv4("192.168.1.42")

	r.RecordBlocked(client, "ads.example.com", "A", "list")
	c.advance(12 * time.Second)
	r.RecordBlocked(client, "ads.example.com", "A", "list")
	c.advance(33 * time.Second)
	r.RecordBlocked(client, "ads.example.com", "A", "list")

	require.NoError(t, r.Flush(t.Context()))
	chunk := store.chunk(t, TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599})

	entry := chunk.Entries[0]
	assert.Equal(t, uint32(3), entry.Count)
	assert.Equal(t, Offsets{0, 12, 45}, entry.Ts)
	assert.True(t, slices.IsSorted(entry.Ts), "offsets must be ascending")
}

func TestRecorder_RotatesOnChunkBoundary(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{ChunkDuration: time.Hour})
	client := types.MustParseIPv4("192.168.1.42")

	r.RecordBlocked(client, "first.example.com", "A", "list")
	c.advance(time.Hour)
	r.RecordBlocked(client, "second.example.com", "A", "list")
	require.NoError(t, r.Flush(t.Context()))

	firstRange := TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599}
	secondRange := TimeRange{Start: firstRange.Start + 3600, End: firstRange.End + 3600}

	first := store.chunk(t, firstRange)
	require.Len(t, first.Entries, 1)
	assert.Equal(t, "first.example.com", first.Entries[0].Domain)

	second := store.chunk(t, secondRange)
	require.Len(t, second.Entries, 1)
	assert.Equal(t, "second.example.com", second.Entries[0].Domain)
	assert.Equal(t, Offsets{0}, second.Entries[0].Ts, "offsets restart at the new chunk")
}

func TestRecorder_EntryCapFoldsIntoOverflow(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{MaxEntries: 3})
	client := types.MustParseIPv4("192.168.1.42")

	for _, domain := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		r.RecordBlocked(client, domain, "A", "list")
	}
	for _, domain := range []string{"d.example.com", "e.example.com"} {
		r.RecordBlocked(client, domain, "A", "list")
	}
	r.RecordBlocked(client, "a.example.com", "A", "list")

	require.NoError(t, r.Flush(t.Context()))
	chunk := store.chunk(t, TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599})

	overflow, ok := findEntry(chunk.Entries, OverflowDomain)
	require.True(t, ok, "entries past the cap must be counted")
	assert.Equal(t, uint32(2), overflow.Count)

	a, ok := findEntry(chunk.Entries, "a.example.com")
	require.True(t, ok)
	assert.Equal(t, uint32(2), a.Count, "an existing key keeps recording past the cap")
	assert.Len(t, chunk.Entries, 4, "three keys plus the overflow entry")
}

func TestRecorder_OffsetCapKeepsCountAuthoritative(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})
	client := types.MustParseIPv4("192.168.1.42")

	const hits = maxOffsets + 500
	for range hits {
		r.RecordBlocked(client, "loop.example.com", "A", "list")
	}

	require.NoError(t, r.Flush(t.Context()))
	chunk := store.chunk(t, TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599})

	entry := chunk.Entries[0]
	assert.Equal(t, uint32(hits), entry.Count, "count must not be capped")
	assert.Len(t, entry.Ts, maxOffsets, "offsets become a sample")
}

func TestRecorder_FlushIsIdempotentWhenIdle(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})

	require.NoError(t, r.Flush(t.Context()))
	assert.Zero(t, store.saves, "nothing recorded means nothing to write")

	r.RecordBlocked(types.MustParseIPv4("192.168.1.42"), "ads.example.com", "A", "list")
	require.NoError(t, r.Flush(t.Context()))
	assert.Equal(t, 1, store.saves)
}

func TestRecorder_RewritesCurrentChunkOnEachFlush(t *testing.T) {
	r, store, c := newTestRecorder(t, Config{})
	client := types.MustParseIPv4("192.168.1.42")
	tr := TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599}

	r.RecordBlocked(client, "ads.example.com", "A", "list")
	require.NoError(t, r.Flush(t.Context()))
	assert.Equal(t, uint32(1), store.chunk(t, tr).Entries[0].Count)

	c.advance(time.Minute)
	r.RecordBlocked(client, "ads.example.com", "A", "list")
	require.NoError(t, r.Flush(t.Context()))
	assert.Equal(t, uint32(2), store.chunk(t, tr).Entries[0].Count)
	assert.Equal(t, 2, store.saves)
}

func TestRecorder_CloseFlushesWithCancelledContext(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})
	r.RecordBlocked(types.MustParseIPv4("192.168.1.42"), "ads.example.com", "A", "list")

	require.NoError(t, r.Close())
	assert.Equal(t, 1, store.saves, "shutdown must not drop unflushed history")
}

func TestRecorder_CloseLeavesStoreOpen(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})
	r.RecordBlocked(types.MustParseIPv4("192.168.1.42"), "ads.example.com", "A", "list")

	require.NoError(t, r.Close())

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, 1, store.saves, "closing must still flush")
	assert.False(t, store.closed, "closing the store is the caller's business")
}

func TestRecorder_ConcurrentRecording(t *testing.T) {
	r, store, _ := newTestRecorder(t, Config{})
	client := types.MustParseIPv4("192.168.1.42")

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for range 200 {
				r.RecordBlocked(client, "ads.example.com", "A", "list")
				r.RecordBlocked(client, "other"+string(rune('a'+i))+".example.com", "A", "list")
			}
		})
	}
	wg.Wait()
	require.NoError(t, r.Flush(t.Context()))

	chunk := store.chunk(t, TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599})
	shared, ok := findEntry(chunk.Entries, "ads.example.com")
	require.True(t, ok)
	assert.Equal(t, uint32(8*200), shared.Count)
	assert.Len(t, chunk.Entries, 9, "one shared domain plus one per goroutine")
}

func TestRecorderConfig_Defaults(t *testing.T) {
	var cfg Config
	cfg.SetDefaults()
	assert.Equal(t, DefaultChunkDuration, cfg.ChunkDuration)
	assert.Equal(t, DefaultFlushInterval, cfg.FlushInterval)
	assert.Equal(t, DefaultMaxEntries, cfg.MaxEntries)

	cfg = Config{ChunkDuration: 48 * time.Hour}
	cfg.SetDefaults()
	assert.Equal(t, time.Duration(maxChunkSeconds)*time.Second, cfg.ChunkDuration)
}

func TestRecorder_WritesThroughFileStore(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, store.Init(t.Context()))
	defer store.Close()

	c := &clock{now: hourStart}
	r := NewRecorder(store, Config{}, slog.New(slog.DiscardHandler))
	r.now = c.Now

	client := types.MustParseIPv4("192.168.1.42")
	r.RecordBlocked(client, "ads.example.com", "A", "hagezi-pro")
	c.advance(time.Second)
	r.RecordBlocked(client, "ads.example.com", "A", "hagezi-pro")
	require.NoError(t, r.Flush(t.Context()))

	tr := TimeRange{Start: Timestamp(hourStart.Unix()), End: Timestamp(hourStart.Unix()) + 3599}
	var got []Entry
	for chunk, err := range store.Load(t.Context(), tr) {
		require.NoError(t, err)
		got = append(got, chunk.Entries...)
	}
	require.Len(t, got, 1)
	assert.Equal(t, "ads.example.com", got[0].Domain)
	assert.Equal(t, uint32(2), got[0].Count)
	assert.Equal(t, Offsets{0, 1}, got[0].Ts)
	assert.NotContains(t, got[0].Domain, "\t")
}

func TestRecorder_FlushKeepsChunksWhoseSaveFailed(t *testing.T) {
	r, store, clk := newTestRecorder(t, Config{})

	store.err = errors.New("disk full")
	r.RecordBlocked(types.MustParseIPv4("192.168.1.1"), "ads.example.com", "A", "list")
	clk.advance(2 * time.Hour)
	r.RecordBlocked(types.MustParseIPv4("192.168.1.1"), "other.example.com", "A", "list")

	require.Error(t, r.Flush(t.Context()))

	store.err = nil
	require.NoError(t, r.Flush(t.Context()))

	var domains []string
	for _, c := range store.chunks {
		for _, e := range c.Entries {
			domains = append(domains, e.Domain)
		}
	}
	assert.Contains(t, domains, "ads.example.com", "a chunk whose save failed must be retried, not dropped")
}
