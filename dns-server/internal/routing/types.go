package routing

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type IPRoute struct {
	Table int        `json:"table"`
	Iface string     `json:"iface"`
	Addr  types.IPv4 `json:"addr"`
}

func (r IPRoute) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("addr", r.Addr.String()),
		slog.String("iface", r.Iface),
	)
}

type IPRouteInfo struct {
	Domain  string          `json:"domain"`
	Reason  string          `json:"reason"`
	AddedAt types.Timestamp `json:"added_at"`
}

type IPRouteDNS struct {
	IPRoute
	IPRouteInfo
	DNSRecord []DNSRecord           `json:"dns_records,omitempty"`
	Lookups   []*types.DomainLookup `json:"lookups,omitempty"`
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
	From     string
	Priority int
}

func (r IPRoutingRule) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("table", r.Table),
		slog.String("from", r.From),
		slog.Int("priority", r.Priority),
	)
}

type DNSRecord struct {
	IP       types.IPv4      `json:"ip"`
	Domain   string          `json:"domain"`
	Resolved types.Timestamp `json:"resolved"`
	Expires  types.Timestamp `json:"expires"`
}

func newDNSRecord(domain string, ip types.IPv4, resolveTime types.Timestamp, ttlSeconds int) DNSRecord {
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
