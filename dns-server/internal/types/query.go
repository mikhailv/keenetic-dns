package types

import (
	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

var _ stream.CursorAware = (*DNSQuery)(nil)

type DNSQuery struct {
	Cursor     stream.Cursor `json:"cursor,omitempty"`
	ID         QueryID       `json:"id"`
	Time       Timestamp     `json:"time"`
	ClientIP   IPv4          `json:"client_ip"`
	Domain     string        `json:"domain"`
	QType      string        `json:"qtype,omitempty"`
	Duration   float64       `json:"duration"`
	ReusedFrom QueryID       `json:"reused_from,omitempty"`
	Blocked    *BlockInfo    `json:"blocked,omitempty"`
	Lookup     *DomainLookup `json:"lookup,omitempty"`
	IPRoutings IPRoutings    `json:"ip_routings,omitempty"`
}

func (s *DNSQuery) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}

// BlockInfo records why a query was answered from the blocklist instead of from upstream. Domain is what the
// blocklist matched, which is the queried domain itself unless the answer pointed at a blocked CNAME.
type BlockInfo struct {
	List    string `json:"list"`
	Domain  string `json:"domain"`
	Pattern string `json:"pattern,omitempty"`
}

var _ stream.CursorAware = (*DNSRawQuery)(nil)

type DNSRawQuery struct {
	Cursor   stream.Cursor `json:"cursor,omitempty"`
	ID       QueryID       `json:"id"`
	Time     Timestamp     `json:"time"`
	ClientIP IPv4          `json:"client_ip"`
	Response bool          `json:"response,omitempty"`
	Msg      PackedMsg     `json:"msg"`
	Error    error         `json:"error,omitempty"`
}

func (s *DNSRawQuery) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}

func (s *DNSRawQuery) String() string {
	if s.Msg != nil {
		return s.Msg.String()
	}
	return "ERROR: " + s.Error.Error()
}

type PackedMsg []byte

func (m PackedMsg) Unpack() *dns.Msg {
	var r dns.Msg
	util.PanicIf(r.Unpack(m))
	return &r
}

func (m PackedMsg) String() string {
	return m.Unpack().String()
}

func (m PackedMsg) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}
