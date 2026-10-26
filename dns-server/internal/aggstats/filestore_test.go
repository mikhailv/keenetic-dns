package aggstats

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hourRange(start Timestamp) TimeRange {
	return TimeRange{Start: start, End: start + 3599}
}

func newTestStore(t *testing.T, normalize func(*testKey)) (Store[testKey, Counter], string) {
	t.Helper()
	dir := t.TempDir()
	s := NewFileStore[testKey, Counter](dir, normalize, slog.New(slog.DiscardHandler))
	require.NoError(t, s.Init(t.Context()))
	t.Cleanup(func() { assert.NoError(t, s.Close()) })
	return s, dir
}

func sampleChunk(start Timestamp) testChunk {
	return testChunk{
		TimeRange: hourRange(start),
		Entries: []testEntry{
			{Key: testKey{Name: "a", QType: "A"}, Value: Counter{Count: 3, Ts: Offsets{12, 45, 3511}}},
			{Key: testKey{Name: "b", QType: "AAAA"}, Value: Counter{Count: 1, Ts: Offsets{900}}},
		},
	}
}

func collect(t *testing.T, s Store[testKey, Counter], tr TimeRange) []testChunk {
	t.Helper()
	var chunks []testChunk
	for chunk, err := range s.Load(t.Context(), tr) {
		require.NoError(t, err)
		chunks = append(chunks, chunk)
	}
	return chunks
}

func TestFileStore_RoundTrip(t *testing.T) {
	s, _ := newTestStore(t, nil)
	want := sampleChunk(1754784000)
	require.NoError(t, s.Save(t.Context(), want))

	chunks := collect(t, s, want.TimeRange)
	require.Len(t, chunks, 1)
	assert.Equal(t, want.TimeRange, chunks[0].TimeRange)
	assert.Equal(t, want.Entries, chunks[0].Entries)
}

func TestFileStore_LoadFiltersByTimeRange(t *testing.T) {
	s, _ := newTestStore(t, nil)
	first := sampleChunk(1754784000)
	second := sampleChunk(1754784000 + 3600)
	third := sampleChunk(1754784000 + 7200)
	for _, c := range []testChunk{first, second, third} {
		require.NoError(t, s.Save(t.Context(), c))
	}

	chunks := collect(t, s, second.TimeRange)
	require.Len(t, chunks, 1)
	assert.Equal(t, second.TimeRange, chunks[0].TimeRange)

	chunks = collect(t, s, TimeRange{Start: first.TimeRange.Start, End: third.TimeRange.End})
	require.Len(t, chunks, 3)
	assert.Equal(t, first.TimeRange.Start, chunks[0].TimeRange.Start)
	assert.Equal(t, third.TimeRange.Start, chunks[2].TimeRange.Start)

	assert.Empty(t, collect(t, s, TimeRange{Start: 1, End: 100}))
}

func TestFileStore_SaveReplacesExistingChunk(t *testing.T) {
	s, _ := newTestStore(t, nil)
	chunk := sampleChunk(1754784000)
	require.NoError(t, s.Save(t.Context(), chunk))

	chunk.Entries[0].Value = Counter{Count: 99, Ts: Offsets{1, 2, 3, 4}}
	require.NoError(t, s.Save(t.Context(), chunk))

	chunks := collect(t, s, chunk.TimeRange)
	require.Len(t, chunks, 1, "re-saving a range must not create a second file")
	assert.Equal(t, uint32(99), chunks[0].Entries[0].Value.Count)
}

func TestFileStore_EmptyChunk(t *testing.T) {
	s, _ := newTestStore(t, nil)
	chunk := testChunk{TimeRange: hourRange(1754784000)}
	require.NoError(t, s.Save(t.Context(), chunk))

	chunks := collect(t, s, chunk.TimeRange)
	require.Len(t, chunks, 1)
	assert.Empty(t, chunks[0].Entries)
}

func TestFileStore_RejectsOverlongChunk(t *testing.T) {
	s, _ := newTestStore(t, nil)
	chunk := testChunk{TimeRange: TimeRange{Start: 1754784000, End: 1754784000 + maxChunkSeconds}}
	assert.Error(t, s.Save(t.Context(), chunk))
}

func TestFileStore_RejectsInvalidTimeRange(t *testing.T) {
	s, _ := newTestStore(t, nil)
	assert.Error(t, s.Save(t.Context(), testChunk{}))
}

func TestFileStore_InitDiscoversExistingChunks(t *testing.T) {
	first, dir := newTestStore(t, nil)
	chunk := sampleChunk(1754784000)
	require.NoError(t, first.Save(t.Context(), chunk))

	second := NewFileStore[testKey, Counter](dir, nil, slog.New(slog.DiscardHandler))
	require.NoError(t, second.Init(t.Context()))
	defer second.Close()

	chunks := collect(t, second, chunk.TimeRange)
	require.Len(t, chunks, 1)
	assert.Equal(t, chunk.Entries, chunks[0].Entries)
}

func TestFileStore_TruncatedFileYieldsPartialData(t *testing.T) {
	s, dir := newTestStore(t, nil)
	chunk := sampleChunk(1754784000)
	require.NoError(t, s.Save(t.Context(), chunk))

	path := filepath.Join(dir, "2025-08-10", "1754784000-1754787599.tsv.gz")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	//nolint:gosec // path built from the test's own temp dir
	require.NoError(t, os.WriteFile(path, data[:len(data)/2], 0o600))

	var got []testChunk
	var loadErr error
	for c, err := range s.Load(t.Context(), chunk.TimeRange) {
		if err != nil {
			loadErr = err
		}
		got = append(got, c)
	}
	require.Error(t, loadErr, "a truncated chunk must report the problem")
	assert.Len(t, got, 1)
}

func TestFileStore_IgnoresUnrelatedFiles(t *testing.T) {
	s, dir := newTestStore(t, nil)
	chunk := sampleChunk(1754784000)
	require.NoError(t, s.Save(t.Context(), chunk))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "2025-08-10", "notes.txt"), []byte("hi"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stray.tsv.gz"), []byte("hi"), 0o600))

	second := NewFileStore[testKey, Counter](dir, nil, slog.New(slog.DiscardHandler))
	require.NoError(t, second.Init(t.Context()))
	defer second.Close()
	assert.Len(t, collect(t, second, chunk.TimeRange), 1)
}

func TestFileStore_NormalizesKeysOnRead(t *testing.T) {
	s, _ := newTestStore(t, func(k *testKey) { k.Name = strings.ToUpper(k.Name) })
	require.NoError(t, s.Save(t.Context(), sampleChunk(1754784000)))

	chunks := collect(t, s, hourRange(1754784000))
	require.Len(t, chunks, 1)
	assert.Equal(t, "A", chunks[0].Entries[0].Key.Name)
}

func TestOffsets_MarshalRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   Offsets
		text string
	}{
		{name: "empty", in: nil, text: ""},
		{name: "single", in: Offsets{0}, text: "0"},
		{name: "several", in: Offsets{12, 45, 3511}, text: "12,45,3511"},
		{name: "max", in: Offsets{65535}, text: "65535"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := tt.in.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, tt.text, string(text))

			var got Offsets
			require.NoError(t, got.UnmarshalText(text))
			assert.Equal(t, tt.in, got)
		})
	}
}

func TestOffsets_UnmarshalRejectsGarbage(t *testing.T) {
	for _, text := range []string{"abc", "12,,45", "12,-1", "65536", "12 45"} {
		var got Offsets
		assert.Error(t, got.UnmarshalText([]byte(text)), text)
	}
}
