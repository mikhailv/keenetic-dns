package types

import (
	"bytes"
	"strconv"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

var timestampMarshalTextCache util.WeakMapVal[Timestamp, []byte]

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

func (t Timestamp) MarshalText() (text []byte, err error) {
	return timestampMarshalTextCache.GetOrCompute(t, func() []byte {
		// 2026-03-19T10:08:13.653Z
		return t.Time().UTC().AppendFormat(make([]byte, 0, 30), time.RFC3339Nano)
	}), nil
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
