package conntrack

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func TestFileStore_SaveLoad(t *testing.T) {
	store := NewFileStore(t.TempDir(), slog.Default())

	base := Timestamp(time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC).Unix())
	tr := TimeRange{Start: base, End: base + 3600}

	chunk := Chunk{
		TimeRange:      tr,
		BucketDuration: 300,
		Buckets: []Bucket{
			{
				TimeRange: TimeRange{Start: base, End: base + 300},
				Entries: []BucketEntry{
					{
						ConnKey: ConnKey{
							Protocol: ProtoTCP,
							SrcIP:    types.MustParseIPv4("192.168.1.1"),
							DstIP:    types.MustParseIPv4("10.0.0.1"),
							DstPort:  443,
						},
						ConnStat: ConnStat{
							BytesOrig:    1234,
							BytesReply:   5678,
							PacketsOrig:  10,
							PacketsReply: 20,
						},
						ConnIDs: []uint32{123},
					},
				},
			},
			{
				TimeRange: TimeRange{Start: base + 300, End: base + 600},
				Entries: []BucketEntry{
					{
						ConnKey: ConnKey{
							Protocol: ProtoUDP,
							SrcIP:    types.MustParseIPv4("192.168.1.2"),
							DstIP:    types.MustParseIPv4("8.8.8.8"),
							DstPort:  53,
						},
						ConnStat: ConnStat{
							BytesOrig:    100,
							BytesReply:   500,
							PacketsOrig:  2,
							PacketsReply: 2,
						},
						ConnIDs: []uint32{123},
					},
				},
			},
		},
	}

	ctx := context.Background()
	require.NoError(t, store.Save(ctx, chunk))

	var loaded []Chunk
	for c, err := range store.Load(ctx, tr) {
		require.NoError(t, err)
		loaded = append(loaded, c)
	}
	require.Len(t, loaded, 1)
	assert.Equal(t, chunk, loaded[0])

	// Range that doesn't intersect — should return nothing.
	var none []Chunk
	for c, err := range store.Load(ctx, TimeRange{Start: base + 10000, End: base + 20000}) {
		require.NoError(t, err)
		none = append(none, c)
	}
	assert.Empty(t, none)
}

func TestFileStore_Init_MigratesFlatFiles(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	base := Timestamp(time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC).Unix())
	tr := TimeRange{Start: base, End: base + 3600}

	chunk := Chunk{
		TimeRange:      tr,
		BucketDuration: 300,
		Buckets: []Bucket{
			{
				TimeRange: TimeRange{Start: base, End: base + 300},
				Entries: []BucketEntry{
					{
						ConnKey: ConnKey{
							Protocol: ProtoTCP,
							SrcIP:    types.MustParseIPv4("192.168.1.1"),
							DstIP:    types.MustParseIPv4("10.0.0.1"),
							DstPort:  443,
						},
						ConnStat: ConnStat{
							BytesOrig:  1234,
							BytesReply: 5678,
						},
						ConnIDs: []uint32{42},
					},
				},
			},
		},
	}

	// Write a chunk file in the old flat layout (directly in dir).
	flatPath := filepath.Join(dir, fmt.Sprintf("%d-%d.bin", tr.Start, tr.End))
	fs := &fileStore{dir: dir, logger: slog.Default()}
	require.NoError(t, fs.saveFile(flatPath, chunk))

	// Verify flat file exists.
	_, err := os.Stat(flatPath)
	require.NoError(t, err)

	// Run Init — should move to date subdir and populate cache.
	store := NewFileStore(dir, slog.Default())
	require.NoError(t, store.Init(ctx))

	// Flat file should be gone.
	_, err = os.Stat(flatPath)
	assert.True(t, os.IsNotExist(err))

	// File should be in date subdir.
	expectedDir := filepath.Join(dir, "2026-04-06")
	entries, err := os.ReadDir(expectedDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, fmt.Sprintf("%d-%d.bin", tr.Start, tr.End), entries[0].Name())

	// Load should work via the initialized cache.
	var loaded []Chunk
	for c, loadErr := range store.Load(ctx, tr) {
		require.NoError(t, loadErr)
		loaded = append(loaded, c)
	}
	require.Len(t, loaded, 1)
	assert.Equal(t, chunk, loaded[0])
}

func TestFileStore_Init_KeepsCorruptChunk(t *testing.T) {
	dir := t.TempDir()

	base := Timestamp(time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC).Unix())
	tr := TimeRange{Start: base, End: base + 3600}

	corrupt := filepath.Join(dir, "2026-04-06", fmt.Sprintf("%d-%d.bin", tr.Start, tr.End))
	require.NoError(t, os.MkdirAll(filepath.Dir(corrupt), 0o755))
	require.NoError(t, os.WriteFile(corrupt, []byte("\x98\x94\x03\x00not a gzip stream"), 0o600))

	good := TimeRange{Start: base + 3601, End: base + 7200}
	store := NewFileStore(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, store.Save(t.Context(), Chunk{TimeRange: good, BucketDuration: 300}))
	require.NoError(t, store.Init(t.Context()))

	_, err := os.Stat(corrupt)
	require.NoError(t, err, "a corrupt chunk must be left in place for the operator to remove")

	var loaded []Chunk
	for c, loadErr := range store.Load(t.Context(), good) {
		require.NoError(t, loadErr)
		loaded = append(loaded, c)
	}
	assert.Len(t, loaded, 1, "the corrupt file must not stop the rest from loading")
}

func TestFileStore_Init_KeepsChunkWithUnknownVersion(t *testing.T) {
	dir := t.TempDir()

	base := Timestamp(time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC).Unix())
	tr := TimeRange{Start: base, End: base + 3600}

	path := filepath.Join(dir, "2026-04-06", fmt.Sprintf("%d-%d.bin", tr.Start, tr.End))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte{encodingVersion + 1})
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))

	store := NewFileStore(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, store.Init(t.Context()))

	_, err = os.Stat(path)
	assert.NoError(t, err, "a chunk from a newer build must be kept")
}

func TestFileStore_Init_IgnoresAbandonedTempFile(t *testing.T) {
	dir := t.TempDir()

	tmp := filepath.Join(dir, "2026-04-06", "1775484000-1775487600.bin.tmp")
	require.NoError(t, os.MkdirAll(filepath.Dir(tmp), 0o755))
	require.NoError(t, os.WriteFile(tmp, []byte("half written"), 0o600))

	store := NewFileStore(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, store.Init(t.Context()))

	tr := TimeRange{Start: 1775484000, End: 1775487600}
	var loaded []Chunk
	for c, err := range store.Load(t.Context(), tr) {
		require.NoError(t, err)
		loaded = append(loaded, c)
	}
	assert.Empty(t, loaded, "a temporary file must not be loaded as a chunk")
}

func TestFileStore_SaveDoesNotWriteInPlace(t *testing.T) {
	dir := t.TempDir()
	fs := &fileStore{dir: dir, logger: slog.New(slog.DiscardHandler)}

	base := Timestamp(time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC).Unix())
	tr := TimeRange{Start: base, End: base + 3600}
	path := filepath.Join(dir, fmt.Sprintf("%d-%d.bin", tr.Start, tr.End))

	require.NoError(t, fs.saveFile(path, Chunk{TimeRange: tr, BucketDuration: 300}))
	first, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, fs.saveFile(path, Chunk{TimeRange: tr, BucketDuration: 600}))
	second, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, os.SameFile(first, second), "save must rename a new file into place, not rewrite the old one")

	chunk, _, err := fs.loadFile(path, false)
	require.NoError(t, err)
	assert.Equal(t, uint(600), chunk.BucketDuration)
}
