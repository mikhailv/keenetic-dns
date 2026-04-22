package internal

import "net"

// Resolver looks up geo information for an IP address.
type Resolver interface {
	Name() string
	Lookup(ip net.IP) (*IPInfo, error)
	Close() error
}
