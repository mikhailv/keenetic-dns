package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/routing"
)

func (s *HTTPServer) handleRoutes(w http.ResponseWriter, req *http.Request) {
	routes := s.ipRoutes.Routes()
	slices.SortFunc(routes, func(a, b routing.IPRouteDNS) int {
		if ap, bp := a.Addr.HasPrefix(), b.Addr.HasPrefix(); ap != bp {
			if ap {
				return 1
			}
			return -1
		}
		return bytes.Compare(a.Addr[:], b.Addr[:])
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(routes) //nolint:errchkjson // ignore any error
}
