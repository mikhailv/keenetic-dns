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
	return IPv4(ip.To4())
}

func (ip IPv4) String() string {
	return net.IP(ip[:]).String()
}

func (ip IPv4) MarshalText() ([]byte, error) {
	return net.IP(ip[:]).MarshalText()
}

func (ip *IPv4) UnmarshalText(b []byte) error {
	var v net.IP
	if err := v.UnmarshalText(b); err != nil {
		return err
	}
	*ip = IPv4(v.To4())
	return nil
}

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

type DomainResolve struct {
	Time   time.Time `json:"time"`
	Domain string    `json:"domain"`
	A      []ARecord `json:"A"`
}

type ARecord struct {
	IP  IPv4 `json:"ip"`
	TTL int  `json:"ttl"`
}

type IPRouteKey struct {
	IP    IPv4
	Iface string
}

type IPRoute struct {
	DNSRecord
	Iface string `json:"iface"`
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
