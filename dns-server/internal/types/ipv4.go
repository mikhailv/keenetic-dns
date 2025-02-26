package types

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type IPv4 [5]byte

func newIPv4(ip net.IP, prefix int) IPv4 {
	ip = ip.To4()
	if len(ip) != 4 {
		panic("invalid IPv4 address")
	}
	if prefix < 0 || prefix > 32 {
		panic("prefix must be between 0 and 32")
	}
	var r IPv4
	copy(r[:], ip)
	r[4] = byte(prefix)
	return r
}

func NewIPv4(ip net.IP) IPv4 {
	return newIPv4(ip.To4(), 32)
}

func ParseIPv4(s string) (IPv4, error) {
	var ip net.IP
	var prefix int
	if p := strings.IndexByte(s, '/'); p < 0 {
		ip = net.ParseIP(s)
		prefix = 32
	} else {
		ip = net.ParseIP(s[:p])
		if n, err := strconv.Atoi(s[p+1:]); err != nil {
			return IPv4{}, fmt.Errorf("failed to parse IP prefix '%s': %w", s[p+1:], err)
		} else {
			prefix = n
		}
	}
	return newIPv4(ip, prefix), nil
}

func (ip IPv4) HasPrefix() bool {
	return ip[4] < 32
}

func (ip IPv4) Prefix() int {
	return int(ip[4])
}

func (ip IPv4) String() string {
	if ip[4] == 32 {
		return net.IP(ip[:4]).String()
	}
	return net.IP(ip[:4]).String() + "/" + strconv.Itoa(int(ip[4]))
}

func (ip IPv4) MarshalText() ([]byte, error) {
	return []byte(ip.String()), nil
}

func (ip *IPv4) UnmarshalText(b []byte) error {
	var err error
	*ip, err = ParseIPv4(string(b))
	return err
}
