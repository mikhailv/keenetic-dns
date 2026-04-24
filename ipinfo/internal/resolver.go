package internal

import (
	"errors"
	"fmt"
	"net"
)

var errSupportedOnlyIPv4 = errors.New("only IPv4 is supported")

// Resolver looks up geo information for an IP address.
type Resolver interface {
	Name() string
	Lookup(ip net.IP, info *IPInfo) error
	Close() error
}

type ResolverRegistry struct {
	defResolver Resolver
	resolvers   map[string]Resolver
}

func NewResolverRegistry(resolvers []Resolver) ResolverRegistry {
	s := ResolverRegistry{
		defResolver: resolvers[0],
		resolvers:   map[string]Resolver{},
	}
	for _, r := range resolvers {
		name := r.Name()
		if _, ok := s.resolvers[name]; ok {
			panic(fmt.Errorf("resolver with name %q already defined", name))
		}
		s.resolvers[name] = r
	}
	return s
}

func (s ResolverRegistry) Get(name string) Resolver {
	if name == "" {
		return s.defResolver
	}
	return s.resolvers[name]
}
