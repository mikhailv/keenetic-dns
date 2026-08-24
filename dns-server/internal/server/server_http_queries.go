package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

var queryStatuses = map[string]func(*types.DNSQuery) bool{
	"reused":   (*types.DNSQuery).Reused,
	"blocked":  (*types.DNSQuery).IsBlocked,
	"routed":   (*types.DNSQuery).Routed,
	"excluded": (*types.DNSQuery).Excluded,
	"direct":   (*types.DNSQuery).Direct,
}

func (s *HTTPServer) filterQueries(_ *http.Request, query url.Values) FilterFunc[types.DNSQuery] {
	domain := strings.TrimSpace(query.Get("domain"))
	search := strings.TrimSpace(query.Get("search"))
	statuses := parseQueryStatuses(query.Get("status"))
	if domain == "" && search == "" && len(statuses) == 0 {
		return nil
	}
	return func(val types.DNSQuery) bool {
		if len(statuses) > 0 && !matchesAnyStatus(&val, statuses) {
			return false
		}
		if search != "" && !strings.Contains(val.Domain, search) {
			return false
		}
		if domain != "" && val.Domain != domain {
			return false
		}
		return true
	}
}

func parseQueryStatuses(value string) []func(*types.DNSQuery) bool {
	var res []func(*types.DNSQuery) bool
	for name := range strings.SplitSeq(value, ",") {
		if match, ok := queryStatuses[strings.TrimSpace(name)]; ok {
			res = append(res, match)
		}
	}
	return res
}

func matchesAnyStatus(val *types.DNSQuery, statuses []func(*types.DNSQuery) bool) bool {
	for _, match := range statuses {
		if match(val) {
			return true
		}
	}
	return false
}
