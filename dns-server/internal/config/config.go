package config

import (
	_ "embed"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
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
	MDNS    MDNS    `yaml:"mdns"`
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
	TTLOverride time.Duration          `yaml:"ttl_override"`
	Providers   map[string]DNSProvider `yaml:"providers"`
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
	DropECH  bool              `yaml:"drop_ech"`
	// one of following must be set
	Endpoint *URL  `yaml:"endpoint"`
	Hosts    Hosts `yaml:"hosts"`
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
	Rule         RoutingRule      `yaml:"rule"`
	Reconcile    RoutingReconcile `yaml:"reconcile"`
	RouteTimeout time.Duration    `yaml:"route_timeout"`
	Hosts        DomainList       `yaml:"hosts"`
	Static       []types.IPv4     `yaml:"static"`
}

type RoutingRule struct {
	Table    int    `yaml:"table"`
	Iif      string `yaml:"iif"`
	Oif      string `yaml:"oif"`
	Priority int    `yaml:"priority"`
}

type RoutingReconcile struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

func (c *Routing) LookupHost(host string) (pattern string) {
	return c.Hosts.Match(host)
}

func (c *Routing) LookupIP(ip types.IPv4) (pattern string) {
	for _, addr := range c.Static {
		if types.PrefixMatch(addr, ip) {
			return addr.String()
		}
	}
	return ""
}

func (c *Config) init() {
	c.setDefaults()
	c.DNS.init()
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

func (c *DNS) init() {
	for name, provider := range c.Providers {
		provider.normalize()
		c.Providers[name] = provider
	}
}

func (c *DNSProvider) normalize() {
	c.Rewrite = normalizeMap(c.Rewrite, func(from string, to string) (string, string) {
		return "." + normalizeFQDN(from), "." + normalizeFQDN(to)
	})
	c.Hosts = normalizeMap(c.Hosts, func(host string, ip net.IP) (string, net.IP) {
		return normalizeFQDN(host), ip
	})
}

//nolint:cyclop // ignore complexity
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
			panic(fmt.Errorf("unexpected port format: %s", s.Port))
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
