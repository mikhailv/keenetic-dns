package config

import (
	_ "embed"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

//go:embed config.default.yaml
var defaultConfigYAML []byte

type Config struct {
	Addr     string `yaml:"addr"`
	HTTPAddr string `yaml:"http_addr"`

	History History `yaml:"history"`
	Agent   Agent   `yaml:"agent"`
	DNS     DNS     `yaml:"dns"`
	Storage Storage `yaml:"storage"`
	Routing Routing `yaml:"routing"`
}

type History struct {
	LogSize      int `yaml:"log_size"`
	DNSQuerySize int `yaml:"dns_query_size"`
}

type Agent struct {
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
}

type DNS struct {
	TTLOverride time.Duration `yaml:"ttl_override"`
	Providers   []DNSProvider `yaml:"providers"`
}

type DNSProvider struct {
	Name     string        `yaml:"name"`
	Priority int           `yaml:"priority"`
	Endpoint URL           `yaml:"endpoint"`
	Ignore   DomainList    `yaml:"ignore"`
	Domains  DomainList    `yaml:"domains"`
	Timeout  time.Duration `yaml:"timeout"`
	Types    []string      `yaml:"types"`
}

type Cache struct {
	Size        int           `yaml:"size"`
	Negative    bool          `yaml:"negative"`
	NegativeTTL time.Duration `yaml:"negative_ttl"`
}

type Storage struct {
	Local *LocalStorage `yaml:"local"`
}

type LocalStorage struct {
	File         string        `yaml:"file"`
	SaveInterval time.Duration `yaml:"save_interval"`
}

type Routing struct {
	Rule           RoutingRule      `yaml:"rule"`
	Reconcile      RoutingReconcile `yaml:"reconcile"`
	RoutingDynamic `yaml:",inline"`
}

type RoutingRule struct {
	Table    int    `yaml:"table"`
	Iif      string `yaml:"iif"`
	Priority int    `yaml:"priority"`
}

type RoutingDynamic struct {
	RouteTimeout time.Duration           `yaml:"route_timeout"`
	Hosts        map[string]DomainList   `yaml:"hosts"`
	Static       map[string][]types.IPv4 `yaml:"static"`
}

type RoutingReconcile struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

func (c *Routing) LookupHost(host string) (iface string) {
	for iface, hosts := range c.Hosts {
		if hosts.Match(host) > 0 {
			return iface
		}
	}
	return ""
}

func (c *Config) init() {
	c.setDefaults()
}

func (c *Config) setDefaults() {
	if c.HTTPAddr == "" {
		c.HTTPAddr = c.Addr
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
