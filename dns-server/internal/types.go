package internal

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/stream"
)

type IPv4 [5]byte

func newIPv4(ip net.IP, prefix int) IPv4 {
	ip = ip.To4()
	if len(ip) != 4 {
		panic("invalid IPv4 address")
	}
	if prefix < 0 || prefix > 33 { // 33 is special case
		panic("prefix must be between 0 and 33")
	}
	var r IPv4
	copy(r[:], ip)
	r[4] = byte(prefix)
	return r
}

func NewIPv4(ip net.IP) IPv4 {
	return newIPv4(ip.To4(), 33)
}

func ParseIPv4(s string) (IPv4, error) {
	var ip net.IP
	var prefix int
	if p := strings.IndexByte(s, '/'); p < 0 {
		ip = net.ParseIP(s)
		prefix = 33
	} else {
		ip = net.ParseIP(s[:p])
		if n, err := strconv.Atoi(s[p+1:]); err != nil {
			return IPv4{}, fmt.Errorf("failed to parse IP prefix '%s': %w", s[p+1:], err)
		} else {
			prefix = n
		}
	}
	return newIPv4(ip, prefix), nil
}

func (ip IPv4) HasPrefix() bool {
	return ip[4] <= 32
}

func (ip IPv4) Prefix() int {
	return min(int(ip[4]), 32)
}

func (ip IPv4) String() string {
	if ip[4] == 33 {
		return net.IP(ip[:4]).String()
	}
	return net.IP(ip[:4]).String() + "/" + strconv.Itoa(int(ip[4]))
}

func (ip IPv4) MarshalText() ([]byte, error) {
	return []byte(ip.String()), nil
}

func (ip *IPv4) UnmarshalText(b []byte) error {
	var err error
	*ip, err = ParseIPv4(string(b))
	return err
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

var _ stream.CursorAware = (*DomainResolve)(nil)

type DomainResolve struct {
	Cursor stream.Cursor `json:"cursor,omitempty"`
	Time   time.Time     `json:"time"`
	Domain string        `json:"domain"`
	TTL    uint32        `json:"ttl"`
	IPs    []IPv4        `json:"ips"`
}

func (s *DomainResolve) SetCursor(cursor stream.Cursor) {
	s.Cursor = cursor
}

type IPRoute struct {
	Table int    `json:"table"`
	Iface string `json:"iface"`
	Addr  IPv4   `json:"addr"`
}

func (r IPRoute) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("addr", r.Addr.String()),
		slog.String("iface", r.Iface),
	)
}

type IPRouteDNS struct {
	IPRoute
	DNSRecord []DNSRecord `json:"dns_records,omitempty"`
}

func (r IPRouteDNS) LogValue() slog.Value {
	attrs := make([]slog.Attr, 0, 2+2*len(r.DNSRecord))
	attrs = append(attrs, slog.String("addr", r.Addr.String()), slog.String("iface", r.Iface))
	for i, rec := range r.DNSRecord {
		if i == 0 {
			attrs = append(attrs, slog.String("domain", rec.Domain), slog.Duration("ttl", rec.TTL()))
		} else {
			attrs = append(attrs, slog.String(fmt.Sprintf("domain%d", i+1), rec.Domain), slog.Duration(fmt.Sprintf("ttl%d", i+1), rec.TTL()))
		}
	}
	return slog.GroupValue(attrs...)
}

type IPRoutingRule struct {
	Table    int
	Iif      string
	Priority int
}

func (r IPRoutingRule) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("table", r.Table),
		slog.String("iif", r.Iif),
		slog.Int("priority", r.Priority),
	)
}
