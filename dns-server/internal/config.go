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
	Addr     string `yaml:"addr"`
	HTTPAddr string `yaml:"http_addr"`

	LogHistorySize      int `yaml:"log_history_size"`
	DNSQueryHistorySize int `yaml:"dns_query_history_size"`

	AgentBaseURL string        `yaml:"agent_base_url"`
	AgentTimeout time.Duration `yaml:"agent_timeout"`

	DNSProvider        string        `yaml:"dns_provider"`
	DNSProviderTimeout time.Duration `yaml:"dns_provider_timeout"`
	DNSTTLOverride     time.Duration `yaml:"dns_ttl_override"`

	ReconcileInterval time.Duration `yaml:"reconcile_interval"`
	ReconcileTimeout  time.Duration `yaml:"reconcile_timeout"`

	MDNS    MDNSConfig    `yaml:"mdns"`
	Dump    DumpConfig    `yaml:"dump"`
	Routing RoutingConfig `yaml:"routing"`
}

type MDNSConfig struct {
	Domains      []string      `yaml:"domains"`
	Addr         string        `yaml:"addr"`
	QueryTimeout time.Duration `yaml:"query_timeout"`
}

type DumpConfig struct {
	File     string        `yaml:"file"`
	Interval time.Duration `yaml:"interval"`
}

type RoutingConfig struct {
	Rule                 RoutingRuleConfig `yaml:"rule"`
	RoutingDynamicConfig `yaml:",inline"`
}

type RoutingDynamicConfig struct {
	RouteTimeout time.Duration     `yaml:"route_timeout"`
	Hosts        map[string]Hosts  `yaml:"hosts"`
	Static       map[string][]IPv4 `yaml:"static"`
}

type RoutingRuleConfig struct {
	Table    int    `yaml:"table"`
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

func (c *Config) init() {
	c.setDefaults()
	c.MDNS.normalize()
}

func (c *Config) setDefaults() {
	if c.HTTPAddr == "" {
		c.HTTPAddr = c.Addr
	}
}

func (c *MDNSConfig) normalize() {
	for i := range c.Domains {
		c.Domains[i] = "." + strings.Trim(c.Domains[i], ".") + "."
	}
}

func DefaultConfig() *Config {
	cfg := defaultConfig()
	cfg.init()
	return cfg
}

func LoadConfig(file string) (*Config, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	cfg := defaultConfig()
	if err = yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	cfg.init()
	return cfg, nil
}

func defaultConfig() *Config {
	var cfg Config
	if err := yaml.Unmarshal(defaultConfigYAML, &cfg); err != nil {
		panic(fmt.Errorf("failed to load default config: %w", err))
	}
	return &cfg
}
