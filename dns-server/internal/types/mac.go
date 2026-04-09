package types

import (
	"encoding/hex"
	"net"
)

// MAC is a 6-byte hardware address stored in binary form.
type MAC [6]byte

// ParseMAC parses a colon-separated MAC string like "aa:bb:cc:dd:ee:ff".
func ParseMAC(s string) MAC {
	var m MAC
	if s == "" {
		return m
	}
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return m
	}
	copy(m[:], hw)
	return m
}

// IsZero returns true if the MAC is all zeros (unset).
func (m MAC) IsZero() bool {
	return m == MAC{}
}

func (m MAC) format() []byte {
	var buf [18]byte
	for i := range m {
		p := i * 3
		hex.Encode(buf[p:p+2], m[i:i+1])
		buf[p+2] = ':'
	}
	return buf[:17]
}

func (m MAC) String() string {
	return string(m.format())
}

// MarshalText implements encoding.TextMarshaler for JSON output.
func (m MAC) MarshalText() ([]byte, error) {
	return m.format(), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *MAC) UnmarshalText(b []byte) error {
	*m = ParseMAC(string(b))
	return nil
}
