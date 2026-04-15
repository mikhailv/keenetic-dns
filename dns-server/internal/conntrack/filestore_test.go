package conntrack

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

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
