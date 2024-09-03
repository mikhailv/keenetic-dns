package log

import (
	"fmt"
	"log/slog"
)

type Entry struct {
	Time  int64             `json:"time"`
	Level string            `json:"level"`
	Msg   string            `json:"msg"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

func NewEntry(rec slog.Record) Entry {
	entry := Entry{
		Time:  rec.Time.UnixMilli(),
		Level: rec.Level.String(),
		Msg:   rec.Message,
	}
	if rec.NumAttrs() > 0 {
		entry.Attrs = make(map[string]string, rec.NumAttrs())
		rec.Attrs(entry.addAttr)
	}
	return entry
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
