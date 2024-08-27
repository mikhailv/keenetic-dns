package internal

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed config.default.yaml
var defaultConfigYAML []byte

type Config struct {
	UDPAddr  string `yaml:"udp_addr"`
	HTTPAddr string `yaml:"http_addr"`

	DNSProvider        string        `yaml:"dns_provider"`
	DNSProviderTimeout time.Duration `yaml:"dns_provider_timeout"`

	ReconcileInterval time.Duration `yaml:"reconcile_interval"`

	Dump    DumpConfig    `yaml:"dump"`
	Routing RoutingConfig `yaml:"routing"`
}

type DumpConfig struct {
	File     string        `yaml:"file"`
	Interval time.Duration `yaml:"interval"`
}

type RoutingConfig struct {
	Table        int               `yaml:"table"`
	Rule         RoutingRuleConfig `yaml:"rule"`
	RouteTimeout time.Duration     `yaml:"route_timeout"`
	Hosts        map[string]Hosts  `yaml:"hosts"`
}

type RoutingRuleConfig struct {
	Iif      string `yaml:"iif"`
	Priority int    `yaml:"priority"`
}

type Hosts []string

func (s Hosts) LookupHost(host string) bool {
	for _, h := range s {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

func (c *RoutingConfig) LookupHost(host string) (iface string) {
	for iface, hosts := range c.Hosts {
		if hosts.LookupHost(host) {
			return iface
		}
	}
	return ""
}

func (c *RoutingConfig) RoutingRule() IPRoutingRule {
	return IPRoutingRule{
		Iif:      c.Rule.Iif,
		TableID:  c.Table,
		Priority: c.Rule.Priority,
	}
}

func DefaultConfig() *Config {
	var cfg Config
	if err := yaml.Unmarshal(defaultConfigYAML, &cfg); err != nil {
		panic(fmt.Errorf("failed to load default config: %w", err))
	}
	return &cfg
}

func LoadConfig(file string) (*Config, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	cfg := DefaultConfig()
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return cfg, nil
}
