package aggstats

import (
	"errors"
	"strconv"
	"strings"
)

const maxOffsets = 4096

type Counter struct {
	Count uint32  `tsv:"count"`
	Ts    Offsets `tsv:"ts,optional"`
}

func (c *Counter) Add(offset uint16) {
	c.Count++
	if len(c.Ts) < maxOffsets {
		c.Ts = append(c.Ts, offset)
	}
}

func (c Counter) Clone() Counter {
	c.Ts = c.Ts.clone()
	return c
}

type Count struct {
	Count uint32 `tsv:"count"`
}

func (c *Count) Add(uint16) {
	c.Count++
}

type Offsets []uint16

var errInvalidOffset = errors.New("aggstats: invalid offset")

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
