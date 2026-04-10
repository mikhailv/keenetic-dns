package types

import (
	"bytes"
	"encoding"
	"strconv"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

var timestampMarshalJSONCache util.WeakMapVal[Timestamp, []byte]

var _ encoding.TextAppender = Timestamp(0)

type Timestamp int64

func TimestampFromTime(t time.Time) Timestamp {
	return Timestamp(t.UnixMilli())
}

func (t Timestamp) Time() time.Time {
	if t == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(t))
}

func (t Timestamp) Add(d time.Duration) Timestamp {
	return TimestampFromTime(t.Time().Add(d))
}

func (t Timestamp) AppendText(b []byte) ([]byte, error) {
	if t == 0 {
		return append(b, "0001-01-01T00:00:00Z"...), nil
	}
	tt := t.Time().UTC()
	year, month, day := tt.Date()
	hours, minutes, seconds := tt.Clock()
	ms := tt.Nanosecond() / 1_000_000

	b = appendInt(b, year, 4)
	b = append(b, '-')
	b = appendInt(b, int(month), 2)
	b = append(b, '-')
	b = appendInt(b, day, 2)
	b = append(b, 'T')
	b = appendInt(b, hours, 2)
	b = append(b, ':')
	b = appendInt(b, minutes, 2)
	b = append(b, ':')
	b = appendInt(b, seconds, 2)
	if ms > 0 {
		b = append(b, '.')
		b = appendInt(b, ms, 3)
	}
	b = append(b, 'Z')
	return b, nil
}

func (t Timestamp) MarshalText() ([]byte, error) {
	return t.AppendText(make([]byte, 0, 24))
}

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return timestampMarshalJSONCache.GetOrCompute(t, func() []byte {
		// `"` + max 24 (2006-01-02T15:04:05.000Z) + `"` = 26
		var buf [26]byte
		buf[0] = '"'
		b, _ := t.AppendText(buf[:1])
		b = append(b, '"')
		return b
	}), nil
}

// appendInt appends v zero-padded to width digits.
func appendInt(b []byte, v int, width int) []byte {
	var tmp [4]byte
	for i := width - 1; i >= 0; i-- {
		tmp[i] = byte('0' + v%10)
		v /= 10
	}
	return append(b, tmp[:width]...)
}

func (t *Timestamp) UnmarshalText(text []byte) error {
	if bytes.ContainsRune(text, 'T') { // parse as RFC3339
		var v time.Time
		if err := v.UnmarshalText(text); err != nil {
			return err
		}
		*t = TimestampFromTime(v)
		return nil
	}

	v, err := strconv.ParseInt(string(text), 10, 64)
	if err != nil {
		return err
	}
	*t = Timestamp(v)
	return nil
}
