package stream

import (
	"fmt"
	"strconv"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

type Cursor uint64

var cursorEpoch = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

const cursorCounterBits uint = 20

type CursorAware interface {
	SetCursor(cursor Cursor)
}

func NewCursor(now time.Time, counter uint64) Cursor {
	return Cursor(util.PackEventID(cursorEpoch, now, cursorCounterBits, counter))
}

func (c Cursor) Time() time.Time {
	return util.EventIDTime(cursorEpoch, cursorCounterBits, uint64(c))
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
