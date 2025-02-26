package server

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/mikhailv/keenetic-dns/internal/log"
)

func (s *HTTPServer) filterLogs(_ *http.Request, query url.Values) FilterFunc[log.Entry] {
	levels := slices.DeleteFunc(strings.Split(query.Get("level"), ","), func(s string) bool { return s == "" })
	if len(levels) == 0 {
		return nil
	}
	levelSet := map[string]bool{}
	for _, level := range levels {
		levelSet[level] = true
	}
	return func(val log.Entry) bool {
		return levelSet[val.Level]
	}
}
