package types

import (
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

var (
	errInvalidIPv4Address = errors.New("invalid IPv4 address")
	errInvalidIPv4Prefix  = errors.New("prefix must be between 0 and 32")

	ipv4MarshalJSONCache util.WeakMapVal[IPv4, []byte]
)

var _ encoding.TextAppender = IPv4{}

type IPv4 [5]byte

func newIPv4(ip net.IP, prefix int) (IPv4, error) {
	ip = ip.To4()
	if len(ip) != 4 {
		return IPv4{}, errInvalidIPv4Address
	}
	if prefix < 0 || prefix > 32 {
		return IPv4{}, errInvalidIPv4Prefix
	}
	var r IPv4
	copy(r[:], ip)
	r[4] = byte(prefix)
	return r, nil
}

func NewIPv4(ip net.IP) IPv4 {
	return util.UnwrapResult(newIPv4(ip.To4(), 32))
}

func ParseIPv4(s string) (IPv4, error) {
	var ip net.IP
	var prefix int
	if before, after, ok := strings.Cut(s, "/"); !ok {
		ip = net.ParseIP(s)
		prefix = 32
	} else {
		ip = net.ParseIP(before)
		if n, err := strconv.Atoi(after); err != nil {
			return IPv4{}, fmt.Errorf("failed to parse IP prefix '%s': %w", after, err)
		} else {
			prefix = n
		}
	}
	return newIPv4(ip, prefix)
}

func MustParseIPv4(s string) IPv4 {
	return util.UnwrapResult(ParseIPv4(s))
}

func (ip IPv4) HasPrefix() bool {
	return ip[4] < 32
}

func (ip IPv4) Prefix() int {
	return int(ip[4])
}

func (ip IPv4) Mask() [4]byte {
	var m [4]byte
	binary.BigEndian.PutUint32(m[:], ^((uint32(1) << (32 - uint32(ip.Prefix()))) - 1))
	return m
}

func (ip IPv4) String() string {
	var buf [20]byte
	b, _ := ip.AppendText(buf[:0])
	return string(b)
}

func (ip IPv4) AppendText(b []byte) ([]byte, error) {
	b = appendDecimalByte(b, ip[0])
	b = append(b, '.')
	b = appendDecimalByte(b, ip[1])
	b = append(b, '.')
	b = appendDecimalByte(b, ip[2])
	b = append(b, '.')
	b = appendDecimalByte(b, ip[3])
	if ip[4] < 32 {
		b = append(b, '/')
		b = appendDecimalByte(b, ip[4])
	}
	return b, nil
}

func (ip IPv4) MarshalText() ([]byte, error) {
	return ip.AppendText(make([]byte, 0, 18))
}

func (ip IPv4) MarshalJSON() ([]byte, error) {
	return ipv4MarshalJSONCache.GetOrCompute(ip, func() []byte {
		// max: `"` + 18 (255.255.255.255/32) + `"` = 20
		var buf [20]byte
		buf[0] = '"'
		b, _ := ip.AppendText(buf[:1])
		b = append(b, '"')
		return b
	}), nil
}

func appendDecimalByte(b []byte, v byte) []byte {
	switch {
	case v >= 100:
		b = append(b, '0'+v/100)
		v %= 100
		b = append(b, '0'+v/10, '0'+v%10)
	case v >= 10:
		b = append(b, '0'+v/10, '0'+v%10)
	default:
		b = append(b, '0'+v)
	}
	return b
}

func (ip *IPv4) UnmarshalText(b []byte) error {
	var err error
	*ip, err = ParseIPv4(string(b))
	return err
}

func PrefixMatch(prefixIP, ip IPv4) bool {
	if ip.HasPrefix() {
		return false
	}
	if !prefixIP.HasPrefix() {
		return prefixIP == ip
	}
	m := prefixIP.Mask()
	for i := range 4 {
		if prefixIP[i]&m[i] != ip[i]&m[i] {
			return false
		}
	}
	return true
}
