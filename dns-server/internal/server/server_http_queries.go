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
	if domain == "" && search == "" && !excludeRouted {
		return nil
	}
	return func(val types.DNSQuery) bool {
		if excludeRouted && s.ipRoutes.LookupHost(val.Domain) != "" {
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
