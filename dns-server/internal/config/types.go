package config

import (
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
		ss[i] = normalizeDomain(ss[i])
	}
	*s = ss
	return nil
}

func (s DomainList) Match(domain string) int {
	if len(s) == 0 {
		return 0
	}
	// TODO: O(N) host lookup, maybe optimize?
	domain = normalizeDomain(domain)
	for _, suffix := range s {
		if strings.HasSuffix(domain, suffix) {
			return len(suffix)
		}
	}
	return -1
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

func normalizeDomain(domain string) string {
	return "." + strings.Trim(domain, ".")
}
