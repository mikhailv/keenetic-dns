package config

import (
	"net"
	"net/url"
	"strings"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/domainstats"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/lookup"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type DomainStats = domainstats.Config

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

type DomainList lookup.DomainTree[string]

func (s *DomainList) UnmarshalYAML(unmarshal func(any) error) error {
	var ss []string
	if err := unmarshal(&ss); err != nil {
		return err
	}
	tb := lookup.NewDomainTreeBuilder[string]()
	for i := range ss {
		tb.Add(ss[i], normalizeFQDN(ss[i]))
	}
	*s = DomainList(tb.Build())
	return nil
}

func (s DomainList) Empty() bool {
	return lookup.DomainTree[string](s).Empty()
}

func (s DomainList) Match(domain string) (pattern string) {
	pattern, _ = lookup.DomainTree[string](s).Get(domain)
	return pattern
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
	return util.FQDN(strings.Trim(domain, "."))
}

func isDomainSuffix(domain string, suffix string) bool {
	return domain == suffix || (strings.HasSuffix(domain, suffix) && domain[len(domain)-len(suffix)-1] == '.')
}
