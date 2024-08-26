package internal

import (
	"log/slog"
	"net"
	"time"
)

type IPv4 [4]byte

func NewIPv4(ip net.IP) IPv4 {
	ip = ip.To4()
	if len(ip) != 4 {
		panic("invalid IPv4 address")
	}
	return IPv4{ip[0], ip[1], ip[2], ip[3]}
}

func (ip IPv4) String() string {
	return net.IP(ip[:]).String()
}

type DNSRecordKey struct {
	IP     IPv4   `json:"ip"`
	Domain string `json:"domain"`
}

type DNSRecord struct {
	DNSRecordKey
	Expires time.Time `json:"expires"`
}

func NewDNSRecord(domain string, ip IPv4, ttl time.Duration) DNSRecord {
	return DNSRecord{DNSRecordKey{ip, domain}, time.Now().Add(ttl)}
}

func (r DNSRecord) Expired() bool {
	return time.Now().After(r.Expires)
}

func (r DNSRecord) TTL() time.Duration {
	if r.Expired() {
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

type IPRouteKey struct {
	IP    IPv4
	Iface string
}

type IPRoute struct {
	DNSRecord
	Iface string
}

func (r IPRoute) Expired(extraTTL time.Duration) bool {
	return time.Now().After(r.Expires.Add(extraTTL))
}

func (r IPRoute) Key() IPRouteKey {
	return IPRouteKey{
		IP:    r.IP,
		Iface: r.Iface,
	}
}

func (r IPRoute) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("domain", r.Domain),
		slog.String("ip", r.IP.String()),
		slog.Duration("ttl", r.TTL()),
		slog.String("iface", r.Iface),
	)
}

type IPRoutingRule struct {
	Iif      string
	TableID  int
	Priority int
}

func (r IPRoutingRule) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("iif", r.Iif),
		slog.Int("table", r.TableID),
		slog.Int("priority", r.Priority),
	)
}
