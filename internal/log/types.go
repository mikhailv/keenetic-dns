package log

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

var _ util.CursorAware = (*Entry)(nil)

type Entry struct {
	Cursor uint64            `json:"cursor,omitempty"`
	Time   time.Time         `json:"time"`
	Level  string            `json:"level"`
	Msg    string            `json:"msg"`
	Attrs  map[string]string `json:"attrs,omitempty"`
}

func NewEntry(rec slog.Record) Entry {
	entry := Entry{
		Time:  rec.Time.UTC(),
		Level: rec.Level.String(),
		Msg:   rec.Message,
	}
	if rec.NumAttrs() > 0 {
		entry.Attrs = make(map[string]string, rec.NumAttrs())
		rec.Attrs(entry.addAttr)
	}
	return entry
}

func (s *Entry) SetCursor(cursor uint64) {
	s.Cursor = cursor
}

func (s *Entry) addAttrs(attrs []slog.Attr) {
	for _, attr := range attrs {
		s.addAttr(attr)
	}
}

func (s *Entry) addAttr(attr slog.Attr) bool {
	val := attr.Value.Resolve().Any()
	if attrs, ok := val.([]slog.Attr); ok {
		s.addAttrs(attrs)
	} else {
		s.Attrs[attr.Key] = fmt.Sprint(val)
	}
	return true
}
