package routing

import (
	"fmt"
	"log/slog"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
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

type IPRouteDNS struct {
	IPRoute
	DNSRecord []types.DNSRecord `json:"dnsRecords,omitempty"`
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

type IPRoutingRule config.RoutingRule

func (r IPRoutingRule) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("table", r.Table),
		slog.String("iif", r.Iif),
		slog.Int("priority", r.Priority),
	)
}
