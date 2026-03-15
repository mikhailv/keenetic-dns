package types

import (
	"log/slog"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type DNSRecordKey struct {
	IP     IPv4   `json:"ip" tsv:"ip"`
	Domain string `json:"domain" tsv:"domain"`
}

type DNSRecord struct {
	DNSRecordKey
	Resolved Timestamp `json:"resolved" tsv:"resolved"`
	Expires  Timestamp `json:"expires" tsv:"expires"`
}

func NewDNSRecord(domain string, ip IPv4, resolveTime Timestamp, ttlSeconds int) DNSRecord {
	return DNSRecord{
		DNSRecordKey: DNSRecordKey{ip, domain},
		Resolved:     resolveTime,
		Expires:      resolveTime.Add(time.Duration(ttlSeconds) * time.Second),
	}
}

func (r DNSRecord) Expired(extraTTL time.Duration) bool {
	return time.Now().After(r.Expires.Time().Add(extraTTL))
}

func (r DNSRecord) TTL() time.Duration {
	if r.Expired(0) {
		return 0
	}
	return time.Until(r.Expires.Time()).Truncate(time.Second)
}

func (r DNSRecord) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("domain", r.Domain),
		slog.String("ip", r.IP.String()),
		slog.Time("resolved", r.Resolved.Time()),
		slog.Duration("ttl", r.TTL()),
	)
}

var _ stream.CursorAware = (*DNSQuery)(nil)

type DNSQuery struct {
	Cursor     stream.Cursor `json:"cursor,omitempty"`
	Time       Timestamp     `json:"time"`
	ClientAddr string        `json:"client_addr"`
	Duration   float64       `json:"duration"`
	DomainLookup
}

func (s *DNSQuery) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}

func (s *DNSQuery) HasRoutedIPs() bool {
	for _, ip := range s.IPs {
		if ip.Routed() {
			return true
		}
	}
	return false
}

type DomainLookup struct {
	Domain string                `json:"domain"`
	CNames []DomainEntry[string] `json:"cnames,omitempty"`
	IPs    []DomainIP            `json:"ips"`
}

type DomainIP struct {
	IP          IPv4                  `json:"ip"`
	TTL         uint32                `json:"ttl"`
	PTR         []DomainEntry[string] `json:"ptr,omitempty"`
	SOA         []DomainEntry[string] `json:"soa,omitempty"`
	RouteAdded  bool                  `json:"route_added,omitempty"`
	RouteIface  string                `json:"route_iface,omitempty"`
	RouteReason string                `json:"route_reason,omitempty"`
}

type DomainEntry[T comparable] struct {
	Name T      `json:"name"`
	TTL  uint32 `json:"ttl"`
}

func (s *DomainLookup) SetRouted(added bool, iface, reason string) {
	for i := range s.IPs {
		s.IPs[i].SetRouted(added, iface, reason)
	}
}

func (s *DomainIP) SetRouted(added bool, iface, reason string) {
	s.RouteAdded = added
	s.RouteIface = iface
	s.RouteReason = reason
}

func (s *DomainIP) Routed() bool {
	return s.RouteIface != ""
}

var _ stream.CursorAware = (*DNSRawQuery)(nil)

type DNSRawQuery struct {
	Cursor     stream.Cursor `json:"cursor,omitempty"`
	Time       Timestamp     `json:"time"`
	ClientAddr string        `json:"client_addr"`
	Response   bool          `json:"response,omitempty"`
	Msg        PackedMsg     `json:"msg"`
	Error      error         `json:"error,omitempty"`
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
