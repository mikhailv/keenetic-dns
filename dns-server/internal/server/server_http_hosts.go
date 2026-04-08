package server

import (
	"net/http"

	"github.com/goccy/go-json"
)

func (s *HTTPServer) handleListHosts(w http.ResponseWriter, req *http.Request) (int, error) {
	hosts, err := s.networkService.ListHosts(req.Context())
	if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hosts)
	return http.StatusOK, nil
}
