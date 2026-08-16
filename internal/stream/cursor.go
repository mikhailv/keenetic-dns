package stream

import (
	"fmt"
	"strconv"
	"time"
)

type Cursor uint64

var cursorEpoch = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

const cursorCounterBits = 20

type CursorAware interface {
	SetCursor(cursor Cursor)
}

func NewCursor(now time.Time, counter uint64) Cursor {
	ms := max(now.Sub(cursorEpoch).Milliseconds(), 0)
	return Cursor(uint64(ms)<<cursorCounterBits | counter&(1<<cursorCounterBits-1))
}

func (c Cursor) Time() time.Time {
	return cursorEpoch.Add(time.Duration(uint64(c)>>cursorCounterBits) * time.Millisecond)
}

func (c Cursor) String() string {
	return fmt.Sprintf("%016x", uint64(c))
}

func (c Cursor) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

func (c *Cursor) UnmarshalText(b []byte) error {
	cur, err := ParseCursor(string(b))
	if err == nil {
		*c = cur
	}
	return err
}

func ParseCursor(s string) (Cursor, error) {
	if n, err := strconv.ParseUint(s, 16, 64); err != nil {
		return 0, err
	} else {
		return Cursor(n), nil
	}
}
