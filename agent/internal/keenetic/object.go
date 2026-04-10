package keenetic

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Object map[string]any

func (s Object) GetObject(prop string) Object {
	v, ok := s.get(prop)
	if !ok {
		return nil
	}
	r, _ := v.(Object)
	return r
}

func (s Object) GetString(prop string) *string {
	v, ok := s.get(prop)
	if !ok {
		return nil
	}
	r, _ := v.(string)
	return &r
}

func (s Object) GetInt(prop string) *int64 {
	v := s.GetString(prop)
	if v == nil {
		return nil
	}
	n, _ := strconv.ParseInt(*v, 10, 64)
	return &n
}

func (s Object) GetBool(prop string) *bool {
	v := s.GetString(prop)
	if v == nil {
		return nil
	}
	r := *v == "yes"
	return &r
}

func (s Object) String() string {
	var sb strings.Builder
	sb.Grow(8 << 10)
	e := json.NewEncoder(&sb)
	e.SetIndent("", "  ")
	if err := e.Encode(s); err != nil {
		return fmt.Sprintf("Object.String() error: %v", err)
	}
	return sb.String()
}

func (s Object) get(prop string) (any, bool) {
	before, after, ok := strings.Cut(prop, ".")
	if !ok {
		v, ok := s[prop]
		return v, ok
	}
	if v, ok := s[before]; ok {
		if c, ok := v.(Object); ok {
			return c.get(after)
		}
	}
	return nil, false
}
