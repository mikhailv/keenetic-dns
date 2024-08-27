package log

import (
	"fmt"
	"log/slog"
)

type Entry struct {
	Time  int64          `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

func NewEntry(rec slog.Record) Entry {
	entry := Entry{
		Time:  rec.Time.UnixMilli(),
		Level: rec.Level.String(),
		Msg:   rec.Message,
	}
	if rec.NumAttrs() > 0 {
		entry.Attrs = make(map[string]any, rec.NumAttrs())
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
	} else if err, ok := val.(error); ok {
		s.Attrs[attr.Key] = err.Error()
	} else if str, ok := val.(fmt.Stringer); ok {
		s.Attrs[attr.Key] = str.String()
	} else {
		s.Attrs[attr.Key] = val
	}
	return true
}
