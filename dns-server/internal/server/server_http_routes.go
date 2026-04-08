package server

import (
	"bytes"
	"net/http"
	"slices"

	"github.com/goccy/go-json"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/routing"
)

func (s *HTTPServer) handleListRoutes(w http.ResponseWriter, req *http.Request) (int, error) {
	query := req.URL.Query()
	routes := s.ipRoutes.Routes(queryParamSet(query, "with_lookups"))
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
	_ = json.NewEncoder(w).Encode(routes)
	return http.StatusOK, nil
}
