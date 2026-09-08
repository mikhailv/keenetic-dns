package conntrack

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type mockAgent struct {
	agentclient.NetworkServiceClient
	entries []agentclient.ConntrackEntry
}

func (m *mockAgent) ListConntrack(_ context.Context) ([]agentclient.ConntrackEntry, error) {
	return m.entries, nil
}

// testStart is an hour boundary, so a test can advance by minutes without rotating the chunk.
var testStart = time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)

func newTestTracker(t *testing.T) (*Tracker, *mockAgent) {
	t.Helper()
	store := NewFileStore(t.TempDir(), slog.Default())
	agent := &mockAgent{}
	tr := NewTracker(Config{
		PollInterval:   time.Second,
		BucketInterval: time.Minute,
		ChunkInterval:  time.Hour,
		CacheDuration:  time.Hour,
	}, slog.Default(), agent, store)
	tr.bucketRange = makeRange(testStart, tr.cfg.BucketInterval)
	tr.chunkRange = makeRange(testStart, tr.cfg.ChunkInterval)
	return tr, agent
}

func TestTracker_PollAndBucket(t *testing.T) {
	ctx := t.Context()

	tracker, agent := newTestTracker(t)
	now := testStart

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol: "tcp",
			SrcIp:    "192.168.1.10",
			DstIp:    "8.8.8.8",
			SrcPort:  new(uint16(12345)),
			DstPort:  new(uint16(443)),
			Id:       new(uint32(92345)),
		},
	}
	tracker.poll(ctx, now.Add(time.Second))

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol:     "tcp",
			SrcIp:        "192.168.1.10",
			DstIp:        "8.8.8.8",
			SrcPort:      new(uint16(12345)),
			DstPort:      new(uint16(443)),
			BytesOrig:    1000,
			BytesReply:   5000,
			PacketsOrig:  10,
			PacketsReply: 20,
			Id:           new(uint32(92345)),
		},
		{
			Protocol:     "tcp",
			SrcIp:        "192.168.1.10",
			DstIp:        "8.8.8.8",
			SrcPort:      new(uint16(12346)),
			DstPort:      new(uint16(443)),
			BytesOrig:    500,
			BytesReply:   2000,
			PacketsOrig:  5,
			PacketsReply: 8,
			Id:           new(uint32(92346)),
		},
	}

	tracker.poll(ctx, now.Add(time.Second))

	ck := ConnKey{
		Protocol: ProtoTCP,
		SrcIP:    types.MustParseIPv4("192.168.1.10"),
		DstIP:    types.MustParseIPv4("8.8.8.8"),
		DstPort:  443,
	}
	assert.Equal(t, map[ConnKey]*bucketEntryAccumulator{
		ck: {
			ConnStat: ConnStat{
				BytesOrig:    1500,
				BytesReply:   7000,
				PacketsOrig:  15,
				PacketsReply: 28,
			},
			ConnIDs: map[uint32]struct{}{92345: {}, 92346: {}},
		},
	}, tracker.bucketEntries)

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol:     "tcp",
			SrcIp:        "192.168.1.10",
			DstIp:        "8.8.8.8",
			SrcPort:      new(uint16(12345)),
			DstPort:      new(uint16(443)),
			BytesOrig:    2000, // +1000
			BytesReply:   8000, // +3000
			PacketsOrig:  15,
			PacketsReply: 30,
			Id:           new(uint32(92345)),
		},
	}

	tracker.poll(ctx, now.Add(2*time.Second))

	assert.Equal(t, map[ConnKey]*bucketEntryAccumulator{
		ck: {
			ConnStat: ConnStat{
				BytesOrig:    2500,
				BytesReply:   10000,
				PacketsOrig:  20,
				PacketsReply: 38,
			},
			ConnIDs: map[uint32]struct{}{92345: {}, 92346: {}},
		},
	}, tracker.bucketEntries)
}

func TestTracker_BucketSeal(t *testing.T) {
	ctx := t.Context()

	tracker, agent := newTestTracker(t)
	now := testStart

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol: "udp",
			SrcIp:    "192.168.1.20",
			DstIp:    "1.1.1.1",
			SrcPort:  new(uint16(5000)),
			DstPort:  new(uint16(53)),
			Id:       new(uint32(100500)),
		},
	}
	tracker.poll(ctx, now.Add(time.Second))

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol:     "udp",
			SrcIp:        "192.168.1.20",
			DstIp:        "1.1.1.1",
			SrcPort:      new(uint16(5000)),
			DstPort:      new(uint16(53)),
			BytesOrig:    100,
			BytesReply:   200,
			PacketsOrig:  1,
			PacketsReply: 1,
			Id:           new(uint32(100500)),
		},
	}
	tracker.poll(ctx, now.Add(time.Second))

	// Advance past the bucket boundary.
	tracker.poll(ctx, now.Add(time.Minute))

	tracker.mu.RLock()
	defer tracker.mu.RUnlock()

	bucketStart := Timestamp(now.Unix())
	assert.Equal(t, map[Timestamp]Bucket{
		bucketStart: {
			TimeRange: TimeRange{Start: bucketStart, End: bucketStart + 59},
			Entries: []BucketEntry{
				{
					ConnKey: ConnKey{
						Protocol: ProtoUDP,
						SrcIP:    types.MustParseIPv4("192.168.1.20"),
						DstIP:    types.MustParseIPv4("1.1.1.1"),
						DstPort:  53,
					},
					ConnStat: ConnStat{
						BytesOrig:    100,
						BytesReply:   200,
						PacketsOrig:  1,
						PacketsReply: 1,
					},
					ConnIDs: []uint32{100500},
				},
			},
		},
	}, tracker.chunkBuckets)
}
