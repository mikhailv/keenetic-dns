package conntrack

import (
	"encoding/hex"
	"fmt"
	"maps"
	"net"
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

// ParseProtocol converts a protocol name to Protocol.
func ParseProtocol(s string) Protocol {
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

// ParseIP parses an IP string into types.IPv4.
func ParseIP(s string) types.IPv4 {
	ip, _ := types.ParseIPv4(s)
	return ip
}

// MAC is a 6-byte hardware address stored in binary form.
type MAC [6]byte

// ParseMAC parses a colon-separated MAC string like "aa:bb:cc:dd:ee:ff".
func ParseMAC(s string) MAC {
	var m MAC
	if s == "" {
		return m
	}
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return m
	}
	copy(m[:], hw)
	return m
}

// IsZero returns true if the MAC is all zeros (unset).
func (m MAC) IsZero() bool {
	return m == MAC{}
}

func (m MAC) format() []byte {
	var buf [18]byte
	for i := range m {
		p := i * 3
		hex.Encode(buf[p:p+2], m[i:i+1])
		buf[p+2] = ':'
	}
	return buf[:18]
}

func (m MAC) String() string {
	return string(m.format())
}

// MarshalText implements encoding.TextMarshaler for JSON output.
func (m MAC) MarshalText() ([]byte, error) {
	return m.format(), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *MAC) UnmarshalText(b []byte) error {
	*m = ParseMAC(string(b))
	return nil
}

// ConnKey identifies a logical connection group (without ephemeral src_port).
type ConnKey struct {
	_        struct{}   `cbor:",toarray"`
	Protocol Protocol   // 1 byte
	SrcIP    types.IPv4 // 5 bytes
	DstIP    types.IPv4 // 5 bytes
	DstPort  uint16     // 2 bytes
	MAC      MAC        // 6 bytes
}

type Chunk struct {
	TimeRange      TimeRange            `json:"time_range"`
	BucketDuration uint                 `json:"bucket_duration"` // seconds
	Buckets        map[Timestamp]Bucket `json:"buckets"`
}

func (c Chunk) Clone() Chunk {
	c.Buckets = maps.Clone(c.Buckets)
	return c
}

// Bucket holds all entries for a single time bucket.
type Bucket struct {
	Cursor    stream.Cursor `cbor:"-" json:"cursor"`
	_         struct{}      `cbor:",toarray"`
	TimeRange TimeRange     `json:"time_range"`
	Entries   []BucketEntry `json:"entries"`
}

func (s *Bucket) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}

// BucketEntry holds aggregated traffic stats for one ConnKey within a time bucket.
type BucketEntry struct {
	_ struct{} `cbor:",toarray"`
	ConnKey
	BytesOrig    int64
	BytesReply   int64
	PacketsOrig  uint32
	PacketsReply uint32
	Connections  uint16
}

// snapshotKey is the full 5-tuple used to track individual connections between polls.
type snapshotKey struct {
	Protocol Protocol
	SrcIP    types.IPv4
	SrcPort  uint16
	DstIP    types.IPv4
	DstPort  uint16
	MAC      MAC
}

// snapshotEntry holds the counters from one poll for a single connection.
type snapshotEntry struct {
	BytesOrig    int64
	BytesReply   int64
	PacketsOrig  uint32
	PacketsReply uint32
}

// bucketAccumulator accumulates deltas for a single ConnKey within the current bucket.
type bucketAccumulator struct {
	bytesOrig    int64
	bytesReply   int64
	packetsOrig  uint32
	packetsReply uint32
	srcPorts     util.Set[uint16]
}

func (a *bucketAccumulator) addDelta(srcPort uint16, delta snapshotEntry) {
	a.bytesOrig += delta.BytesOrig
	a.bytesReply += delta.BytesReply
	a.packetsOrig += delta.PacketsOrig
	a.packetsReply += delta.PacketsReply
	a.srcPorts.Add(srcPort)
}

func (a *bucketAccumulator) toBucketEntry(key ConnKey) BucketEntry {
	return BucketEntry{
		ConnKey:      key,
		BytesOrig:    a.bytesOrig,
		BytesReply:   a.bytesReply,
		PacketsOrig:  a.packetsOrig,
		PacketsReply: a.packetsReply,
		Connections:  uint16(len(a.srcPorts)),
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
