package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func (s *HTTPServer) filterRawQueries(_ *http.Request, query url.Values) FilterFunc[types.DNSRawQuery] {
	search := strings.TrimSpace(query.Get("search"))
	onlyResponses := queryParamSet(query, "only_responses")
	if search == "" && !onlyResponses {
		return nil
	}
	return func(val types.DNSRawQuery) bool {
		if onlyResponses && !val.Response {
			return false
		}
		if search != "" && !strings.Contains(val.Text, search) {
			return false
		}
		return true
	}
}
