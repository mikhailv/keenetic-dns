package conntrack

import (
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

// Protocol represents a network protocol as its IANA protocol number.
type Protocol uint8

const (
	ProtoICMP Protocol = 1
	ProtoTCP  Protocol = 6
	ProtoUDP  Protocol = 17
)

func (p Protocol) String() string {
	switch p {
	case ProtoTCP:
		return "tcp"
	case ProtoUDP:
		return "udp"
	case ProtoICMP:
		return "icmp"
	default:
		return "unknown"
	}
}

func (p Protocol) MarshalText() ([]byte, error) {
	return util.StringToBytes(p.String()), nil
}

// parseProtocol converts a protocol name to Protocol.
func parseProtocol(s string) Protocol {
	switch s {
	case "tcp":
		return ProtoTCP
	case "udp":
		return ProtoUDP
	case "icmp":
		return ProtoICMP
	default:
		return 0
	}
}

// parseIP parses an IP string into types.IPv4.
func parseIP(s string) types.IPv4 {
	ip, _ := types.ParseIPv4(s)
	return ip
}

// ConnKey identifies a logical connection group (without ephemeral src_port).
type ConnKey struct {
	_        struct{}   `cbor:",toarray"`
	Protocol Protocol   `json:"protocol"` // 1 byte
	SrcIP    types.IPv4 `json:"src_ip"`   // 5 bytes
	DstIP    types.IPv4 `json:"dst_ip"`   // 5 bytes
	DstPort  uint16     `json:"dst_port"` // 2 bytes
}

type Chunk struct {
	TimeRange      TimeRange `json:"time_range"`
	BucketDuration uint      `json:"bucket_duration"` // seconds
	Buckets        []Bucket  `json:"buckets"`
}

func (s *Chunk) IsValid() bool {
	return !s.TimeRange.IsZero() && s.BucketDuration > 0
}

func (s *Chunk) Clone() Chunk {
	c := *s
	c.Buckets = make([]Bucket, len(s.Buckets))
	for i, bucket := range s.Buckets {
		c.Buckets[i] = bucket.Clone()
	}
	return c
}

// Bucket holds all entries for a single time bucket.
type Bucket struct {
	Cursor    stream.Cursor `cbor:"-" json:"cursor"`
	_         struct{}      `cbor:",toarray"`
	TimeRange TimeRange     `json:"time_range"`
	Entries   []BucketEntry `json:"entries"`
}

func (s *Bucket) Clone() Bucket {
	c := *s
	c.Entries = make([]BucketEntry, len(s.Entries))
	for i, entry := range s.Entries {
		c.Entries[i] = entry.Clone()
	}
	return c
}

func (s *Bucket) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}

// BucketEntry holds aggregated traffic stats for one ConnKey within a time bucket.
type BucketEntry struct {
	_ struct{} `cbor:",toarray"`
	ConnKey
	ConnStat
	SrcPorts []uint16 `json:"src_ports"`
}

func (s *BucketEntry) Clone() BucketEntry {
	c := *s
	c.SrcPorts = slices.Clone(s.SrcPorts)
	return c
}

type ConnStat struct {
	BytesOrig    int64  `json:"bytes_orig"`
	BytesReply   int64  `json:"bytes_reply"`
	PacketsOrig  uint32 `json:"packets_orig"`
	PacketsReply uint32 `json:"packets_reply"`
}

func (s *ConnStat) IsZero() bool {
	return *s == (ConnStat{})
}

func (s *ConnStat) Add(other ConnStat) {
	s.BytesOrig += other.BytesOrig
	s.BytesReply += other.BytesReply
	s.PacketsOrig += other.PacketsOrig
	s.PacketsReply += other.PacketsReply
}

func (s *ConnStat) IsNextFor(prev ConnStat) bool {
	sameOrig := s.BytesOrig == prev.BytesOrig && s.PacketsOrig == prev.PacketsOrig
	sameReply := s.BytesReply == prev.BytesReply && s.PacketsReply == prev.PacketsReply
	biggerOrig := s.BytesOrig > prev.BytesOrig && s.PacketsOrig > prev.PacketsOrig
	biggerReply := s.BytesReply > prev.BytesReply && s.PacketsReply > prev.PacketsReply
	return (sameOrig || biggerOrig) && (sameReply || biggerReply)
}

func (s *ConnStat) IsLikelyNew() bool {
	const bytesThreshold = 128 * 1024
	return s.BytesOrig+s.BytesReply <= bytesThreshold
}

// snapshotKey is the full 5-tuple used to track individual connections between polls.
type snapshotKey struct {
	ConnKey
	SrcPort uint16
}

func (s snapshotKey) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("proto", s.Protocol.String()),
		slog.String("src", s.SrcIP.String()),
		slog.Int("sport", int(s.SrcPort)),
		slog.String("dst", s.DstIP.String()),
		slog.Int("dport", int(s.DstPort)),
	)
}

// snapshotEntry holds the counters from one poll for a single connection.
type snapshotEntry struct {
	ConnStat
	State     string
	MissCount uint8
}

func (s snapshotEntry) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int64("bytes_orig", s.BytesOrig),
		slog.Int64("bytes_reply", s.BytesReply),
		slog.Int64("packets_orig", int64(s.PacketsOrig)),
		slog.Int64("packets_reply", int64(s.PacketsReply)),
		slog.String("state", s.State),
		slog.Int("miss_count", int(s.MissCount)),
	)
}

// bucketEntryAccumulator accumulates deltas for a single ConnKey within the current bucket.
type bucketEntryAccumulator struct {
	ConnStat
	SrcPorts util.Set[uint16]
}

func (s *bucketEntryAccumulator) toBucketEntry(key ConnKey) BucketEntry {
	srcPorts := s.SrcPorts.Values()
	slices.Sort(srcPorts)
	return BucketEntry{
		ConnKey:  key,
		ConnStat: s.ConnStat,
		SrcPorts: srcPorts,
	}
}

type Timestamp uint32

func (t Timestamp) Time() time.Time {
	return time.Unix(int64(t), 0)
}

type TimeRange struct {
	_     struct{}  `cbor:",toarray"`
	Start Timestamp `json:"start"`
	End   Timestamp `json:"end"`
}

func (s TimeRange) IsZero() bool {
	return s == (TimeRange{})
}

func (s TimeRange) StartTime() time.Time {
	return s.Start.Time()
}

func (s TimeRange) EndTime() time.Time {
	return s.End.Time()
}

func (s TimeRange) InRange(t Timestamp) bool {
	return s.Start <= t && t <= s.End
}

func (s TimeRange) Intersects(other TimeRange) bool {
	return s.Start <= other.End && s.End >= other.Start
}

func (s TimeRange) Valid() bool {
	return s.Start <= s.End && s.End > 0
}

func (s TimeRange) String() string {
	return fmt.Sprintf("%d-%d", s.Start, s.End)
}

func makeRange(t time.Time, d time.Duration) TimeRange {
	start := t.Truncate(d)
	return TimeRange{
		Start: Timestamp(start.Unix()),
		End:   Timestamp(start.Add(d).Unix() - 1),
	}
}
