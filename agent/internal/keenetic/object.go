package keenetic

import (
	"encoding/json"
	"strconv"
	"strings"
)

type Object map[string]any

func (s Object) GetObject(prop string) Object {
	v, _ := s.get(prop).(Object)
	return v
}

func (s Object) GetString(prop string) string {
	v, _ := s.get(prop).(string)
	return v
}

func (s Object) GetInt(prop string) int64 {
	v := s.GetString(prop)
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

func (s Object) GetBool(prop string) bool {
	return s.GetString(prop) == "yes"
}

func (s Object) String() string {
	var sb strings.Builder
	sb.Grow(8 << 10)
	e := json.NewEncoder(&sb)
	e.SetIndent("", "  ")
	if err := e.Encode(s); err != nil {
		panic(err)
	}
	return sb.String()
}

func (s Object) get(prop string) any {
	p := strings.Index(prop, ".")
	if p == -1 {
		return s[prop]
	}
	if v, ok := s[prop[:p]]; ok {
		if c, ok := v.(Object); ok {
			return c.get(prop[p+1:])
		}
	}
	return nil
}
