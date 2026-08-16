package config

import (
	"bytes"
	"cmp"
	_ "embed"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/blocklist"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/lookup"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

//go:embed config.default.yaml
var defaultConfigYAML []byte

type Config struct {
	Addr     string `yaml:"addr"`
	HTTPAddr string `yaml:"http_addr"`

	Logging    Logging     `yaml:"logging"`
	Agent      Agent       `yaml:"agent"`
	DNS        DNS         `yaml:"dns"`
	MDNS       MDNS        `yaml:"mdns"`
	Storage    Storage     `yaml:"storage"`
	Routing    Routing     `yaml:"routing"`
	Conntrack  Conntrack   `yaml:"conntrack"`
	Blocking   Blocking    `yaml:"blocking"`
	QueryStats DomainStats `yaml:"query_stats"`
}

type Blocking struct {
	blocklist.Config `yaml:",inline"`
	Stats            DomainStats `yaml:"stats"`
}

func (c *Blocking) SetDefaults() {
	c.Config.SetDefaults()
	c.Stats.SetDefaults()
}

func (c *Blocking) Validate() error {
	return cmp.Or(
		c.Config.Validate(),
		c.Stats.Validate("blocking stats"),
	)
}

type Logging struct {
	Debug       bool `yaml:"debug"`
	HistorySize int  `yaml:"history_size"`
}

type Agent struct {
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
}

type DNS struct {
	TTLOverride      time.Duration          `yaml:"ttl_override"`
	DropECH          bool                   `yaml:"drop_ech"`
	DropAAAA         bool                   `yaml:"drop_aaaa"`
	QueryHistorySize int                    `yaml:"query_history_size"`
	Providers        map[string]DNSProvider `yaml:"providers"`
}

type MDNS struct {
	Enabled  bool          `yaml:"enabled"`
	Services []MDNSService `yaml:"services"`
}

type MDNSService struct {
	Name    string            `yaml:"name"`
	Host    string            `yaml:"host"`
	IP      []net.IP          `yaml:"ip"`
	Port    int               `yaml:"port"`
	Service string            `yaml:"service"`
	TXT     map[string]string `json:"txt"`
}

type DNSProvider struct {
	Enabled bool `yaml:"enabled"`
	// Priority allows to specify order of providers to resolve request, higher values represent higher priority
	Priority int               `yaml:"priority"`
	Ignore   DomainList        `yaml:"ignore"`
	Domains  DomainList        `yaml:"domains"`
	Rewrite  map[string]string `yaml:"rewrite"`
	Timeout  time.Duration     `yaml:"timeout"`
	Types    []string          `yaml:"types"`
	// one of following must be set
	Endpoint *URL  `yaml:"endpoint"`
	Hosts    Hosts `yaml:"hosts"`
}

type Storage struct {
	Local *LocalStorage `yaml:"local"`
}

type LocalStorage struct {
	File         string        `yaml:"file"`
	SaveInterval time.Duration `yaml:"save_interval"`
}

type Routing struct {
	Table        int              `yaml:"table"`
	Oif          string           `yaml:"oif"`
	Rules        []RoutingRule    `yaml:"rules"`
	Reconcile    RoutingReconcile `yaml:"reconcile"`
	RouteTimeout time.Duration    `yaml:"route_timeout"`
	Hosts        DomainList       `yaml:"hosts"`
	Ignore       DomainList       `yaml:"ignore"`
	Static       []types.IPv4     `yaml:"static"`

	staticLookup lookup.IPTree[string]
}

type RoutingRule struct {
	From     string `yaml:"from"`
	Priority int    `yaml:"priority"`
}

type RoutingReconcile struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

type Conntrack struct {
	PollInterval   time.Duration `yaml:"poll_interval"`
	BucketInterval time.Duration `yaml:"bucket_interval"`
	ChunkInterval  time.Duration `yaml:"chunk_interval"`
	CacheDuration  time.Duration `yaml:"cache_duration"`
	SaveInterval   time.Duration `yaml:"save_interval"`
	DataDir        string        `yaml:"data_dir"`
	HistorySize    int           `yaml:"history_size"`
}

func (c *Routing) LookupHost(host string) (pattern string) {
	return c.Hosts.Match(host)
}

func (c *Routing) LookupIgnoredHost(host string) (pattern string) {
	return c.Ignore.Match(host)
}

func (c *Routing) LookupIP(ip types.IPv4) (pattern string) {
	if ip.HasPrefix() {
		return ""
	}
	pattern, _ = c.staticLookup.Get(ip)
	return pattern
}

func (c *Config) init() {
	c.setDefaults()
	c.DNS.init()
	c.Routing.init()
}

func (c *Config) Validate() error {
	return cmp.Or(
		c.Blocking.Validate(),
		c.QueryStats.Validate("query stats"),
	)
}

func (c *Config) setDefaults() {
	if c.HTTPAddr == "" {
		c.HTTPAddr = c.Addr
	}
	c.Blocking.SetDefaults()
	c.QueryStats.SetDefaults()
}

func DefaultConfig() *Config {
	cfg, err := defaultConfig()
	if err != nil {
		panic(err)
	}
	cfg.init()
	return cfg
}

func LoadConfig(file string) (*Config, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	cfg, err := defaultConfig()
	if err != nil {
		return nil, err
	}
	if err = newYAMLDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	cfg.init()
	return cfg, nil
}

func defaultConfig() (*Config, error) {
	cfg := &Config{}
	if err := newYAMLDecoder(bytes.NewReader(defaultConfigYAML)).Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to load default config: %w", err)
	}
	return cfg, nil
}

func newYAMLDecoder(r io.Reader) *yaml.Decoder {
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	return decoder
}

func (c *DNS) init() {
	for name, provider := range c.Providers {
		provider.normalize()
		c.Providers[name] = provider
	}
}

func (c *Routing) init() {
	tb := lookup.NewIPTreeBuilder[string]()
	for _, addr := range c.Static {
		tb.Add(addr, addr.String())
	}
	c.staticLookup = tb.Build()
}

func (c *DNSProvider) normalize() {
	c.Rewrite = normalizeMap(c.Rewrite, func(from string, to string) (string, string) {
		return "." + normalizeFQDN(from), "." + normalizeFQDN(to)
	})
	c.Hosts = normalizeMap(c.Hosts, func(host string, ip net.IP) (string, net.IP) {
		return normalizeFQDN(host), ip
	})
}

func (c *MDNSService) UnmarshalYAML(unmarshal func(any) error) error {
	var s struct {
		Name    string            `yaml:"name"`
		Host    string            `yaml:"host"`
		IP      List[net.IP]      `yaml:"ip"`
		Port    string            `yaml:"port"`
		Service string            `yaml:"service"`
		TXT     map[string]string `json:"txt"`
	}
	if err := unmarshal(&s); err != nil {
		return err
	}

	*c = MDNSService{
		Name:    s.Name,
		Host:    s.Host,
		IP:      s.IP,
		Service: s.Service,
		TXT:     s.TXT,
	}

	if s.Port != "" {
		ss := strings.Split(s.Port, ":")
		if len(ss) > 2 {
			return fmt.Errorf("unexpected port format: %s", s.Port)
		}

		portName := ss[0]
		var portNum string
		if len(ss) == 2 {
			portNum = ss[1]
		} else if _, err := strconv.Atoi(portName); err == nil {
			portNum = portName
			portName = ""
		}

		switch portName {
		case "http":
			portNum = or(portNum, "80")
			c.Service = or(c.Service, "_http._tcp")
		case "ssh":
			portNum = or(portNum, "22")
			c.Service = or(c.Service, "_ssh._tcp")
		case "":
		default:
			return fmt.Errorf("unexpected port name: %s", portName)
		}

		port, err := strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("unexpected port number: %s", portNum)
		}
		c.Port = port
	}
	return nil
}

func or(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func normalizeMap[K comparable, V any, M ~map[K]V](m M, normalize func(key K, value V) (K, V)) M {
	r := make(map[K]V, len(m))
	for k, v := range m {
		k, v = normalize(k, v)
		r[k] = v
	}
	return r
}
