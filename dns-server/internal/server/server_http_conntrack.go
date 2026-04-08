package server

import (
	"net/http"
	"net/url"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
)

func (s *HTTPServer) filterConntrack(_ *http.Request, query url.Values) FilterFunc[conntrack.Bucket] {
	return func(val conntrack.Bucket) bool {
		return true
	}
}
