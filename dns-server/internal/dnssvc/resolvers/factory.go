package resolvers

import (
	"fmt"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

func NewFromConfig(name string, cfg config.DNSProvider) (dnssvc.Provider, error) {
	if (cfg.Endpoint == nil) == (len(cfg.Hosts) == 0) {
		return nil, fmt.Errorf("exactly one of 'endpoint' or 'hosts' property for DNS provider %q must be provided", name)
	}

	var resolver dnssvc.Resolver
	if len(cfg.Hosts) > 0 {
		resolver = NewStaticHostResolver(name+" (static)", cfg.Hosts, time.Minute)
	} else {
		switch cfg.Endpoint.Scheme {
		case "http", "https":
			resolver = NewDoHClient(name+" (DoH)", cfg.Endpoint.String(), cfg.Timeout)
		case "dns", "dns+udp":
			resolver = NewDNSClient(name+" (udp)", "udp", cfg.Endpoint.Host, cfg.Timeout)
		case "dns+tcp":
			resolver = NewDNSClient(name+" (tcp)", "tcp", cfg.Endpoint.Host, cfg.Timeout)
		case "mdns":
			resolver = NewMDNSClient(name+" (mDNS)", cfg.Endpoint.Host, cfg.Timeout)
		}
	}
	return dnssvc.NewProvider(resolver, cfg)
}
