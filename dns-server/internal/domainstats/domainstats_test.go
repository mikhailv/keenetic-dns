package domainstats

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/aggstats"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func hourRange(start aggstats.Timestamp) aggstats.TimeRange {
	return aggstats.TimeRange{Start: start, End: start + 3599}
}

func newTestStore(t *testing.T) (Store, string) {
	t.Helper()
	dir := t.TempDir()
	s := NewFileStore(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, s.Init(t.Context()))
	t.Cleanup(func() { assert.NoError(t, s.Close()) })
	return s, dir
}

func sampleChunk() Chunk {
	return Chunk{
		TimeRange: hourRange(1754784000),
		Entries: []Entry{
			{
				Key:   Key{ClientIP: types.MustParseIPv4("192.168.1.42"), Domain: "ads.example.com.", QType: "A", Label: "hagezi-pro"},
				Value: aggstats.Counter{Count: 3, Ts: aggstats.Offsets{12, 45, 3511}},
			},
			{
				Key:   Key{ClientIP: types.MustParseIPv4("192.168.1.50"), Domain: "tracker.example.net.", QType: "AAAA", Label: "hagezi-pro"},
				Value: aggstats.Counter{Count: 1, Ts: aggstats.Offsets{900}},
			},
		},
	}
}

func TestFileStore_FileFormatIsUnchanged(t *testing.T) {
	s, dir := newTestStore(t)
	require.NoError(t, s.Save(t.Context(), sampleChunk()))

	f, err := os.Open(filepath.Join(dir, "2025-08-10", "1754784000-1754787599.tsv.gz"))
	require.NoError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(bufio.NewReader(f))
	require.NoError(t, err)
	defer gz.Close()

	var lines []string
	sc := bufio.NewScanner(gz)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	require.NoError(t, sc.Err())

	require.Len(t, lines, 3, "header plus one line per entry")
	assert.Equal(t, "client_ip\tdomain\tqtype\tlabel\tcount\tts", lines[0], "files written before the generic store stay readable")
	assert.Equal(t, "192.168.1.42\tads.example.com.\tA\thagezi-pro\t3\t12,45,3511", lines[1])
}

func TestFileStore_ReadsDomainsWrittenWithoutTrailingDot(t *testing.T) {
	s, _ := newTestStore(t)
	chunk := sampleChunk()
	chunk.Entries[0].Key.Domain = "ads.example.com"
	require.NoError(t, s.Save(t.Context(), chunk))

	for c, err := range s.Load(t.Context(), chunk.TimeRange) {
		require.NoError(t, err)
		assert.Equal(t, "ads.example.com.", c.Entries[0].Key.Domain)
	}
}

func TestRecorder_RecordsDomainKeys(t *testing.T) {
	s, _ := newTestStore(t)
	r := NewRecorder(s, aggstats.Config{}, slog.New(slog.DiscardHandler))

	client := types.MustParseIPv4("192.168.1.42")
	r.Record(client, "Ads.Example.COM", "A", "hagezi-pro")
	r.Record(client, "ads.example.com.", "A", "hagezi-pro")
	require.NoError(t, r.Close())

	var entries []Entry
	for c, err := range s.Load(t.Context(), aggstats.TimeRange{Start: 1, End: 1<<32 - 1}) {
		require.NoError(t, err)
		entries = append(entries, c.Entries...)
	}
	require.Len(t, entries, 2, "domains are kept as recorded, apart from the trailing dot")
	for _, e := range entries {
		assert.Equal(t, client, e.Key.ClientIP)
		assert.Equal(t, "hagezi-pro", e.Key.Label)
		assert.Equal(t, uint32(1), e.Value.Count)
	}
}
