package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func (s *HTTPServer) filterQueries(_ *http.Request, query url.Values) FilterFunc[types.DNSQuery] {
	domain := strings.TrimSpace(query.Get("domain"))
	search := strings.TrimSpace(query.Get("search"))
	excludeRouted := queryParamSet(query, "exclude_routed")
	excludeBlocked := queryParamSet(query, "exclude_blocked")
	onlyBlocked := queryParamSet(query, "blocked")
	if domain == "" && search == "" && !excludeRouted && !excludeBlocked && !onlyBlocked {
		return nil
	}
	return func(val types.DNSQuery) bool {
		if excludeRouted && val.IPRoutings.Has(types.ActionRouted) {
			return false
		}
		if excludeBlocked && val.Blocked != nil {
			return false
		}
		if onlyBlocked && val.Blocked == nil {
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
