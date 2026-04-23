package internal

import (
	"errors"
	"net"
)

var errSupportedOnlyIPv4 = errors.New("only IPv4 is supported")

// Resolver looks up geo information for an IP address.
type Resolver interface {
	Name() string
	Lookup(ip net.IP, info *IPInfo) error
	Close() error
}
