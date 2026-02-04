package config

import (
	"net"
	"net/url"
	"strings"
)

type Hosts map[string]net.IP

type List[T any] []T

func (v *List[T]) UnmarshalYAML(unmarshal func(any) error) error {
	var s T
	if err := unmarshal(&s); err == nil {
		*v = []T{s}
	} else {
		var ss []T
		if err = unmarshal(&ss); err != nil {
			return err
		}
		*v = ss
	}
	return nil
}

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

func (s DomainList) Match(domain string) (pattern string) {
	if len(s) == 0 {
		return ""
	}
	// TODO: O(N) host lookup, maybe optimize?
	domain = normalizeFQDN(domain)
	for _, suffix := range s {
		if isDomainSuffix(domain, suffix) {
			return suffix
		}
	}
	return ""
}

type URL url.URL

func (u *URL) String() string {
	if u == nil {
		return "<nil>"
	}
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
