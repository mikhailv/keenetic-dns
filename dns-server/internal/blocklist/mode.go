package blocklist

import "fmt"

type Mode uint8

const (
	ModeNXDomain Mode = iota
	ModeNull
	ModeNoData
)

func (m Mode) String() string {
	switch m {
	case ModeNXDomain:
		return "nxdomain"
	case ModeNull:
		return "null"
	case ModeNoData:
		return "nodata"
	default:
		return "unknown"
	}
}

func (m Mode) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

func (m *Mode) UnmarshalText(text []byte) error {
	mode, err := ParseMode(string(text))
	if err != nil {
		return err
	}
	*m = mode
	return nil
}

func ParseMode(s string) (Mode, error) {
	switch s {
	case "", "nxdomain":
		return ModeNXDomain, nil
	case "null":
		return ModeNull, nil
	case "nodata":
		return ModeNoData, nil
	default:
		return ModeNXDomain, fmt.Errorf("unknown block mode %q", s)
	}
}
