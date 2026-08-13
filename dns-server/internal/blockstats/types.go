package blockstats

import (
	"errors"
	"strconv"
	"strings"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type (
	TimeRange = conntrack.TimeRange
	Timestamp = conntrack.Timestamp
)

const maxChunkSeconds = 1 << 16

const maxOffsets = 4096

type Chunk struct {
	TimeRange TimeRange
	Entries   []Entry
}

type Entry struct {
	ClientIP types.IPv4 `tsv:"client_ip"`
	Domain   string     `tsv:"domain"`
	QType    string     `tsv:"qtype"`
	List     string     `tsv:"list,optional"`
	Count    uint32     `tsv:"count"`
	Ts       Offsets    `tsv:"ts,optional"`
}

type Key struct {
	ClientIP types.IPv4
	Domain   string
	QType    string
	List     string
}

func (e *Entry) Key() Key {
	return Key{ClientIP: e.ClientIP, Domain: e.Domain, QType: e.QType, List: e.List}
}

type Offsets []uint16

var errInvalidOffset = errors.New("blockstats: invalid offset")

func (o Offsets) clone() Offsets {
	if len(o) == 0 {
		return nil
	}
	out := make(Offsets, len(o))
	copy(out, o)
	return out
}

func (o Offsets) MarshalText() ([]byte, error) {
	if len(o) == 0 {
		return nil, nil
	}
	buf := make([]byte, 0, len(o)*6)
	for i, v := range o {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendUint(buf, uint64(v), 10)
	}
	return buf, nil
}

func (o *Offsets) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "" {
		*o = nil
		return nil
	}
	res := make(Offsets, 0, strings.Count(s, ",")+1)
	for field := range strings.SplitSeq(s, ",") {
		v, err := strconv.ParseUint(field, 10, 16)
		if err != nil {
			return errors.Join(errInvalidOffset, err)
		}
		res = append(res, uint16(v))
	}
	*o = res
	return nil
}
