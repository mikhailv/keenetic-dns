package conntrack

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func TestChunkEncoding_ConnIndexDeltas(t *testing.T) {
	base := Timestamp(time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC).Unix())
	key := ConnKey{Protocol: ProtoUDP, SrcIP: types.MustParseIPv4("192.168.2.91"), DstIP: types.MustParseIPv4("192.168.2.1"), DstPort: 53}
	chunk := Chunk{
		TimeRange:      TimeRange{Start: base, End: base + 3599},
		BucketDuration: 60,
		Buckets: []Bucket{
			{
				TimeRange: TimeRange{Start: base, End: base + 59},
				Entries: []BucketEntry{
					{ConnKey: key, ConnStat: ConnStat{BytesOrig: 60, PacketsOrig: 1}, ConnIDs: []uint64{1_792_087_200_000, 1_792_087_200_001, 1_792_087_200_007}},
				},
			},
			{
				TimeRange: TimeRange{Start: base + 60, End: base + 119},
				Entries: []BucketEntry{
					{ConnKey: key, ConnIDs: []uint64{1_792_087_100_123, 1_792_087_200_007}},
					{ConnKey: ConnKey{Protocol: ProtoTCP, SrcIP: key.SrcIP, DstIP: key.DstIP, DstPort: 443}, ConnIDs: []uint64{}},
				},
			},
		},
		ConnState: ConnState{
			ConnIndex: 1_792_087_200_008,
			Conns: []LiveConn{
				{ID: 3_000_000_001, Index: 1_792_087_100_123},
				{ID: 17, Index: 1_792_087_200_007},
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, encodeChunk(&buf, chunk))
	var decoded Chunk
	version, err := decodeChunk(&buf, &decoded, false)
	require.NoError(t, err)
	assert.Equal(t, byte(encodingVersion), version)
	assert.Equal(t, chunk, decoded)
}

func encodeChunkV3(t *testing.T, chunk Chunk) []byte {
	t.Helper()
	var raw bytes.Buffer
	uv := func(v uint64) { raw.Write(binary.AppendUvarint(nil, v)) }
	u32 := func(v uint32) { raw.Write(binary.LittleEndian.AppendUint32(nil, v)) }
	entries, ids := 0, 0
	for _, b := range chunk.Buckets {
		entries += len(b.Entries)
		for _, e := range b.Entries {
			ids += len(e.ConnIDs)
		}
	}
	raw.WriteByte(3)
	uv(uint64(entries))
	uv(uint64(ids))
	u32(uint32(chunk.TimeRange.Start))
	u32(uint32(chunk.TimeRange.End))
	uv(uint64(chunk.BucketDuration))
	uv(uint64(len(chunk.Buckets)))
	for _, b := range chunk.Buckets {
		var bb bytes.Buffer
		bb.Write(binary.LittleEndian.AppendUint32(nil, uint32(b.TimeRange.Start)))
		bb.Write(binary.LittleEndian.AppendUint32(nil, uint32(b.TimeRange.End)))
		bb.Write(binary.AppendUvarint(nil, uint64(len(b.Entries))))
		for _, e := range b.Entries {
			bb.WriteByte(byte(e.Protocol))
			bb.Write(e.SrcIP[:4])
			bb.Write(e.DstIP[:4])
			bb.Write(binary.LittleEndian.AppendUint16(nil, e.DstPort))
			for _, v := range []uint64{uint64(e.BytesOrig), uint64(e.BytesReply), uint64(e.PacketsOrig), uint64(e.PacketsReply), uint64(len(e.ConnIDs))} {
				bb.Write(binary.AppendUvarint(nil, v))
			}
			for _, id := range e.ConnIDs {
				bb.Write(binary.LittleEndian.AppendUint32(nil, uint32(id)))
			}
		}
		uv(uint64(bb.Len()))
		raw.Write(bb.Bytes())
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	_, err := gz.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return out.Bytes()
}

func TestMigrate_V3ToConnIndices(t *testing.T) {
	dir := t.TempDir()
	start := Timestamp(time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC).Unix())
	key := ConnKey{Protocol: ProtoTCP, SrcIP: types.MustParseIPv4("192.168.2.15"), DstIP: types.MustParseIPv4("1.1.1.1"), DstPort: 443}
	other := ConnKey{Protocol: ProtoUDP, SrcIP: types.MustParseIPv4("192.168.2.91"), DstIP: types.MustParseIPv4("192.168.2.1"), DstPort: 53}

	hour := func(i int) TimeRange {
		s := start + Timestamp(i*3600)
		return TimeRange{Start: s, End: s + 3599}
	}
	chunks := []Chunk{
		{TimeRange: hour(0), BucketDuration: 60, Buckets: []Bucket{
			{TimeRange: TimeRange{Start: hour(0).Start, End: hour(0).Start + 59}, Entries: []BucketEntry{
				{ConnKey: key, ConnIDs: []uint64{900, 77}},
			}},
			{TimeRange: TimeRange{Start: hour(0).End - 59, End: hour(0).End}, Entries: []BucketEntry{
				{ConnKey: key, ConnIDs: []uint64{77}},
			}},
		}},
		{TimeRange: hour(1), BucketDuration: 60, Buckets: []Bucket{
			{TimeRange: TimeRange{Start: hour(1).Start, End: hour(1).Start + 59}, Entries: []BucketEntry{
				{ConnKey: key, ConnIDs: []uint64{77, 78}},
				{ConnKey: other, ConnIDs: []uint64{77}},
			}},
		}},
	}
	for _, c := range chunks {
		path := filepath.Join(dir, c.TimeRange.Start.Time().UTC().Format(time.DateOnly), fmt.Sprintf("%d-%d.bin", c.TimeRange.Start, c.TimeRange.End))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, encodeChunkV3(t, c), 0o600))
	}

	store := newFileStore(dir, slog.New(slog.DiscardHandler))
	pending, err := store.scan(t.Context(), dir)
	require.NoError(t, err)
	require.NoError(t, store.migrateFiles(t.Context(), pending))

	load := func(i int) Chunk {
		c, version, err := store.loadFile(store.ChunkPath(hour(i)), false)
		require.NoError(t, err)
		assert.Equal(t, byte(encodingVersion), version)
		return c
	}
	first, second := load(0), load(1)

	b0 := uint64(hour(0).Start) * connIndexPerSecond
	b1 := uint64(hour(1).Start) * connIndexPerSecond
	assert.Equal(t, []uint64{b0, b0 + 1}, first.Buckets[0].Entries[0].ConnIDs, "first-seen order, sorted per entry")
	assert.Equal(t, []uint64{b0 + 1}, first.Buckets[1].Entries[0].ConnIDs, "same connection keeps its index within a chunk")
	assert.Equal(t, []uint64{b0 + 1, b1}, second.Buckets[0].Entries[0].ConnIDs, "a connection continuing from the previous chunk keeps its index")
	assert.Equal(t, []uint64{b1 + 1}, second.Buckets[0].Entries[1].ConnIDs, "the same raw ID under another key is another connection")
}

func TestFileStore_Init_MigratesInBackground(t *testing.T) {
	dir := t.TempDir()
	start := Timestamp(time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC).Unix())
	tr := TimeRange{Start: start, End: start + 3599}
	key := ConnKey{Protocol: ProtoTCP, SrcIP: types.MustParseIPv4("192.168.2.15"), DstIP: types.MustParseIPv4("1.1.1.1"), DstPort: 443}
	chunk := Chunk{TimeRange: tr, BucketDuration: 60, Buckets: []Bucket{
		{TimeRange: TimeRange{Start: start, End: start + 59}, Entries: []BucketEntry{{ConnKey: key, ConnIDs: []uint64{77}}}},
	}}
	path := filepath.Join(dir, "2026-10-15", fmt.Sprintf("%d-%d.bin", tr.Start, tr.End))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, encodeChunkV3(t, chunk), 0o600))

	store := newFileStore(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, store.Init(t.Context()))

	var loaded []Chunk
	for c, err := range store.Load(t.Context(), tr) {
		require.NoError(t, err)
		loaded = append(loaded, c)
	}
	require.Len(t, loaded, 1, "a chunk waiting for migration is still served")
	assert.Len(t, loaded[0].Buckets[0].Entries[0].ConnIDs, 1)

	<-store.scanDone
	_, version, err := store.loadFile(path, true)
	require.NoError(t, err)
	assert.Equal(t, byte(encodingVersion), version)
}

func TestFileStore_Init_DropsCorruptChunkFromIndex(t *testing.T) {
	dir := t.TempDir()
	base := Timestamp(time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC).Unix())
	bad := TimeRange{Start: base, End: base + 3599}
	good := TimeRange{Start: base + 3600, End: base + 7199}

	corrupt := filepath.Join(dir, "2026-04-06", fmt.Sprintf("%d-%d.bin", bad.Start, bad.End))
	require.NoError(t, os.MkdirAll(filepath.Dir(corrupt), 0o755))
	require.NoError(t, os.WriteFile(corrupt, []byte("not a gzip stream"), 0o600))

	store := newFileStore(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, store.Save(t.Context(), Chunk{TimeRange: good, BucketDuration: 60}))
	require.NoError(t, store.Init(t.Context()))
	<-store.scanDone

	var loaded []Chunk
	for c, err := range store.Load(t.Context(), TimeRange{Start: bad.Start, End: good.End}) {
		require.NoError(t, err, "a corrupt chunk found by the background scan must not break loads")
		loaded = append(loaded, c)
	}
	assert.Len(t, loaded, 1)
}

func TestMigrate_SkipsChunkRewrittenSinceScan(t *testing.T) {
	dir := t.TempDir()
	start := Timestamp(time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC).Unix())
	tr := TimeRange{Start: start, End: start + 3599}
	key := ConnKey{Protocol: ProtoTCP, SrcIP: types.MustParseIPv4("192.168.2.15"), DstIP: types.MustParseIPv4("1.1.1.1"), DstPort: 443}
	bucket := func(ids ...uint64) []Bucket {
		return []Bucket{{TimeRange: TimeRange{Start: start, End: start + 59}, Entries: []BucketEntry{{ConnKey: key, ConnIDs: ids}}}}
	}
	path := filepath.Join(dir, "2026-10-15", fmt.Sprintf("%d-%d.bin", tr.Start, tr.End))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, encodeChunkV3(t, Chunk{TimeRange: tr, BucketDuration: 60, Buckets: bucket(77)}), 0o600))

	store := newFileStore(dir, slog.New(slog.DiscardHandler))
	pending, err := store.scan(t.Context(), dir)
	require.NoError(t, err)
	require.Len(t, pending, 1)

	rewritten := Chunk{TimeRange: tr, BucketDuration: 60, Buckets: bucket(1_792_087_200_000, 1_792_087_200_001)}
	require.NoError(t, store.SaveFile(path, rewritten))
	require.NoError(t, store.migrateFiles(t.Context(), pending))

	got, _, err := store.loadFile(path, false)
	require.NoError(t, err)
	assert.Equal(t, rewritten, got, "a chunk saved as v4 after the scan must not be migrated again")
}

func TestTracker_RestoresConnIndexAfterRestart(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	cfg := Config{PollInterval: time.Second, BucketInterval: time.Minute, ChunkInterval: time.Hour, DataDir: dir}

	entry := agentclient.ConntrackEntry{
		Protocol: "tcp",
		SrcIp:    "192.168.1.10",
		DstIp:    "8.8.8.8",
		SrcPort:  new(uint16(12345)),
		DstPort:  new(uint16(443)),
		Id:       new(uint32(92345)),
	}

	agent := &mockAgent{entries: []agentclient.ConntrackEntry{entry}}
	first := NewTracker(cfg, slog.New(slog.DiscardHandler), agent, NewFileStore(dir, slog.New(slog.DiscardHandler)))
	first.loadConnState(ctx)
	first.poll(ctx, testStart.Add(time.Second))
	sk := snapshotKey{
		ConnKey: ConnKey{Protocol: ProtoTCP, SrcIP: types.MustParseIPv4("192.168.1.10"), DstIP: types.MustParseIPv4("8.8.8.8"), DstPort: 443},
		SrcPort: 12345,
		ID:      92345,
	}
	idx := first.prevSnapshot[sk].Index
	assert.GreaterOrEqual(t, idx, uint64(time.Now().Unix())*connIndexPerSecond, "a fresh counter starts from the clock")
	first.saveChunk(ctx, first.snapshotChunkToSave())

	other := entry
	other.SrcPort = new(uint16(12346))
	other.Id = new(uint32(92346))
	agent.entries = []agentclient.ConntrackEntry{entry, other}
	second := NewTracker(cfg, slog.New(slog.DiscardHandler), agent, NewFileStore(dir, slog.New(slog.DiscardHandler)))
	second.loadConnState(ctx)
	second.poll(ctx, testStart.Add(2*time.Second))

	assert.Equal(t, idx, second.prevSnapshot[sk].Index, "an open connection keeps its index across a restart")
	otherKey := sk
	otherKey.SrcPort = 12346
	otherKey.ID = 92346
	assert.Greater(t, second.prevSnapshot[otherKey].Index, idx, "a connection unknown to the saved state gets a new index")
}
