package config

import (
	"net"
	"net/url"
	"strings"
)

type DomainList []string

func (s *DomainList) UnmarshalYAML(unmarshal func(any) error) error {
	var ss []string
	if err := unmarshal(&ss); err != nil {
		return err
	}
	for i := range ss {
		ss[i] = normalizeFQDN(ss[i])
	}
	*s = ss
	return nil
}

func (s DomainList) Match(domain string) int {
	if len(s) == 0 {
		return 0
	}
	// TODO: O(N) host lookup, maybe optimize?
	domain = normalizeFQDN(domain)
	for _, suffix := range s {
		if isDomainSuffix(domain, suffix) {
			return len(suffix)
		}
	}
	return -1
}

type Hosts map[string]net.IP

func (s *Hosts) UnmarshalYAML(unmarshal func(any) error) error {
	var data map[string]net.IP
	if err := unmarshal(&data); err != nil {
		return err
	}
	*s = make(Hosts, len(data))
	hosts := *s
	for domain, ip := range data {
		domain = normalizeFQDN(domain)
		if _, ok := hosts[domain]; ok {
			panic("dns.hosts: duplicate domain " + domain)
		}
		hosts[domain] = ip
	}
	return nil
}

type URL url.URL

func (u *URL) String() string {
	return (*url.URL)(u).String()
}

func (u *URL) UnmarshalText(b []byte) error {
	p, err := url.Parse(string(b))
	if err != nil {
		return err
	}
	*u = URL(*p)
	return nil
}

func (u *URL) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

func normalizeFQDN(domain string) string {
	if !strings.HasPrefix(domain, ".") && strings.HasSuffix(domain, ".") {
		return domain
	}
	return strings.Trim(domain, ".") + "."
}

func isDomainSuffix(domain string, suffix string) bool {
	return domain == suffix || (strings.HasSuffix(domain, suffix) && domain[len(domain)-len(suffix)-1] == '.')
}
