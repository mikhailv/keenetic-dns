package stream

import (
	"fmt"
	"strconv"
)

type Cursor uint64

type CursorAware interface {
	SetCursor(cursor Cursor)
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
