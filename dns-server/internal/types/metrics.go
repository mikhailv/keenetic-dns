package types

import "github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"

func init() {
	metrics.TrackWeakMapSize("IPv4.MarshalText", ipv4MarshalTextCache.Size)
	metrics.TrackWeakMapSize("Timestamp.MarshalText", timestampMarshalTextCache.Size)
}
