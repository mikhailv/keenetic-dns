package types

import (
	"log/slog"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/stream"
)

type DNSRecordKey struct {
	IP     IPv4   `json:"ip"`
	Domain string `json:"domain"`
}

type DNSRecord struct {
	DNSRecordKey
	Expires time.Time `json:"expires"`
}

func NewDNSRecord(domain string, ip IPv4, expires time.Time) DNSRecord {
	return DNSRecord{DNSRecordKey{ip, domain}, expires}
}

func (r DNSRecord) Expired(extraTTL time.Duration) bool {
	if r.Expires.IsZero() {
		return false
	}
	return time.Now().After(r.Expires.Add(extraTTL))
}

func (r DNSRecord) TTL() time.Duration {
	if r.Expired(0) {
		return 0
	}
	return time.Until(r.Expires).Truncate(time.Second)
}

func (r DNSRecord) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("domain", r.Domain),
		slog.String("ip", r.IP.String()),
		slog.Duration("ttl", r.TTL()),
	)
}

var _ stream.CursorAware = (*DNSQuery)(nil)

type DNSQuery struct {
	Cursor     stream.Cursor `json:"cursor,omitempty"`
	Time       time.Time     `json:"time"`
	ClientAddr string        `json:"client_addr"`
	Domain     string        `json:"domain"`
	TTL        uint32        `json:"ttl"`
	IPs        []ResolvedIP  `json:"ips"`
}

func (s *DNSQuery) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}

func (s *DNSQuery) HasRoutedIPs() bool {
	for _, ip := range s.IPs {
		if ip.IsRouted() {
			return true
		}
	}
	return false
}

type ResolvedIP struct {
	IP          IPv4   `json:"ip"`
	RouteIface  string `json:"route_iface,omitempty"`
	RouteReason string `json:"route_reason,omitempty"`
}

func (s *ResolvedIP) IsRouted() bool {
	return s.RouteIface != ""
}

var _ stream.CursorAware = (*DNSRawQuery)(nil)

type DNSRawQuery struct {
	Cursor     stream.Cursor `json:"cursor,omitempty"`
	Time       time.Time     `json:"time"`
	ClientAddr string        `json:"client_addr"`
	Response   bool          `json:"response,omitempty"`
	Text       string        `json:"text"`
}

func (s *DNSRawQuery) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}
