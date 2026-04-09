package conntrack

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type mockAgent struct {
	agentclient.NetworkServiceClient
	entries []agentclient.ConntrackEntry
}

func (m *mockAgent) ListConntrack(_ context.Context) ([]agentclient.ConntrackEntry, error) {
	return m.entries, nil
}

func newTestTracker(t *testing.T) (*Tracker, *mockAgent) {
	t.Helper()
	store := NewFileStore(t.TempDir())
	agent := &mockAgent{}
	tr := NewTracker(TrackerConfig{
		PollInterval:   time.Second,
		BucketInterval: time.Minute,
		ChunkInterval:  time.Hour,
		CacheDuration:  time.Hour,
	}, slog.Default(), agent, store, stream.NewBufferedStream[Bucket](1))
	return tr, agent
}

func TestTracker_PollAndBucket(t *testing.T) {
	ctx := t.Context()

	tracker, agent := newTestTracker(t)
	now := time.Now().Truncate(time.Minute)

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol: "tcp",
			SrcIp:    "192.168.1.10",
			DstIp:    "8.8.8.8",
			SrcPort:  util.Ptr(uint16(12345)),
			DstPort:  util.Ptr(uint16(443)),
		},
	}
	tracker.poll(ctx, now.Add(time.Second))

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol:     "tcp",
			SrcIp:        "192.168.1.10",
			DstIp:        "8.8.8.8",
			SrcPort:      util.Ptr(uint16(12345)),
			DstPort:      util.Ptr(uint16(443)),
			BytesOrig:    1000,
			BytesReply:   5000,
			PacketsOrig:  10,
			PacketsReply: 20,
		},
		{
			Protocol:     "tcp",
			SrcIp:        "192.168.1.10",
			DstIp:        "8.8.8.8",
			SrcPort:      util.Ptr(uint16(12346)), // different src_port, same ConnKey
			DstPort:      util.Ptr(uint16(443)),
			BytesOrig:    500,
			BytesReply:   2000,
			PacketsOrig:  5,
			PacketsReply: 8,
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
			SrcPorts: map[uint16]struct{}{12345: {}, 12346: {}},
		},
	}, tracker.bucketEntries)

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol:     "tcp",
			SrcIp:        "192.168.1.10",
			DstIp:        "8.8.8.8",
			SrcPort:      util.Ptr(uint16(12345)),
			DstPort:      util.Ptr(uint16(443)),
			BytesOrig:    2000, // +1000
			BytesReply:   8000, // +3000
			PacketsOrig:  15,
			PacketsReply: 30,
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
			SrcPorts: map[uint16]struct{}{12345: {}, 12346: {}},
		},
	}, tracker.bucketEntries)
}

func TestTracker_BucketSeal(t *testing.T) {
	ctx := t.Context()

	tracker, agent := newTestTracker(t)
	now := time.Now().Truncate(time.Minute)

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol: "udp",
			SrcIp:    "192.168.1.20",
			DstIp:    "1.1.1.1",
			SrcPort:  util.Ptr(uint16(5000)),
			DstPort:  util.Ptr(uint16(53)),
		},
	}
	tracker.poll(ctx, now.Add(time.Second))

	agent.entries = []agentclient.ConntrackEntry{
		{
			Protocol:     "udp",
			SrcIp:        "192.168.1.20",
			DstIp:        "1.1.1.1",
			SrcPort:      util.Ptr(uint16(5000)),
			DstPort:      util.Ptr(uint16(53)),
			BytesOrig:    100,
			BytesReply:   200,
			PacketsOrig:  1,
			PacketsReply: 1,
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
					SrcPorts: []uint16{5000},
				},
			},
		},
	}, tracker.chunkBuckets)
}
