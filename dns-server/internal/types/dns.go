package types

import (
	"log/slog"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type DNSRecord struct {
	IP       IPv4      `json:"ip"`
	Domain   string    `json:"domain"`
	Resolved Timestamp `json:"resolved"`
	Expires  Timestamp `json:"expires"`
}

func NewDNSRecord(domain string, ip IPv4, resolveTime Timestamp, ttlSeconds int) DNSRecord {
	return DNSRecord{
		IP:       ip,
		Domain:   domain,
		Resolved: resolveTime,
		Expires:  resolveTime.Add(time.Duration(ttlSeconds) * time.Second),
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
	ClientAddr string        `json:"client_addr"`
	Duration   float64       `json:"duration"`
	IPRoutings IPRoutings    `json:"ip_routings,omitempty"`
	DomainLookup
}

func (s *DNSQuery) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}

type DomainLookup struct {
	Time     Timestamp             `json:"time"`
	Resolver ResolverInfo          `json:"resolver"`
	Domain   string                `json:"domain"`
	CNames   []DomainEntry[string] `json:"cnames,omitempty"`
	IPs      []DomainIP            `json:"ips"`
}

func (s *DomainLookup) Expired(extraTTL time.Duration) bool {
	ageSeconds := int(time.Since(s.Time.Time()).Seconds() - extraTTL.Seconds())
	for _, ip := range s.IPs {
		if ip.Expired(ageSeconds) {
			return true
		}
	}
	return false
}

type DomainIP struct {
	IP          IPv4                  `json:"ip"`
	TTL         uint32                `json:"ttl"`
	PTR         []DomainEntry[string] `json:"ptr,omitempty"`
	SOA         []DomainEntry[string] `json:"soa,omitempty"`
	PTRResolver ResolverInfo          `json:"ptr_resolver"`
}

func (s *DomainIP) Expired(ageSeconds int) bool {
	if int(s.TTL) <= ageSeconds {
		return true
	}
	for _, it := range s.PTR {
		if it.Expired(ageSeconds) {
			return true
		}
	}
	for _, it := range s.SOA {
		if it.Expired(ageSeconds) {
			return true
		}
	}
	return false
}

type DomainEntry[T comparable] struct {
	Name T      `json:"name"`
	TTL  uint32 `json:"ttl"`
}

func (s *DomainEntry[T]) Expired(ageSeconds int) bool {
	return int(s.TTL) <= ageSeconds
}

type ResolverInfo struct {
	Name     string  `json:"name"`
	Duration float64 `json:"duration"`
}

type RoutingAction string

const (
	ActionRouted  RoutingAction = "routed"
	ActionIgnored RoutingAction = "ignored"
)

type IPRouting struct {
	IP     IPv4          `json:"ip"`
	Action RoutingAction `json:"action"`
	Static bool          `json:"static"`
	Iface  string        `json:"iface,omitempty"`
	Reason string        `json:"reason"`
	Added  bool          `json:"added,omitempty"`
}

type IPRoutings map[IPv4]IPRouting

func (s *IPRoutings) Has(action RoutingAction) bool {
	for _, it := range *s {
		if it.Action == action {
			return true
		}
	}
	return false
}

func (s *IPRoutings) AddRoute(iface, reason string, ip IPv4) {
	s.add(ActionRouted, false, iface, reason, ip)
}

func (s *IPRoutings) AddStaticRoute(iface, reason string, ip IPv4) {
	s.add(ActionRouted, true, iface, reason, ip)
}

func (s *IPRoutings) AddIgnored(reason string, ip IPv4) {
	s.add(ActionIgnored, false, "", reason, ip)
}

func (s *IPRoutings) add(action RoutingAction, static bool, iface, reason string, ip IPv4) {
	if *s == nil {
		*s = IPRoutings{}
	}
	(*s)[ip] = IPRouting{
		IP:     ip,
		Action: action,
		Static: static,
		Iface:  iface,
		Reason: reason,
	}
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
