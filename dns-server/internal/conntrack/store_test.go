package conntrack

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func TestFileStore_SaveLoad(t *testing.T) {
	store := NewFileStore(t.TempDir())

	base := Timestamp(time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC).Unix())
	tr := TimeRange{Start: base, End: base + 3600}

	chunk := Chunk{
		TimeRange:      tr,
		BucketDuration: 300,
		Buckets: map[Timestamp]Bucket{
			base: {
				TimeRange: TimeRange{Start: base, End: base + 300},
				Entries: []BucketEntry{
					{
						ConnKey: ConnKey{
							Protocol: ProtoTCP,
							SrcIP:    types.MustParseIPv4("192.168.1.1"),
							DstIP:    types.MustParseIPv4("10.0.0.1"),
							DstPort:  443,
							MAC:      ParseMAC("aa:bb:cc:dd:ee:ff"),
						},
						BytesOrig:    1234,
						BytesReply:   5678,
						PacketsOrig:  10,
						PacketsReply: 20,
						Connections:  3,
					},
				},
			},
			base + 300: {
				TimeRange: TimeRange{Start: base + 300, End: base + 600},
				Entries: []BucketEntry{
					{
						ConnKey: ConnKey{
							Protocol: ProtoUDP,
							SrcIP:    types.MustParseIPv4("192.168.1.2"),
							DstIP:    types.MustParseIPv4("8.8.8.8"),
							DstPort:  53,
						},
						BytesOrig:    100,
						BytesReply:   500,
						PacketsOrig:  2,
						PacketsReply: 2,
						Connections:  1,
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
