package server

import (
	"encoding/json"
	"net/http"
)

func (s *HTTPServer) handleListHosts(w http.ResponseWriter, req *http.Request) (int, error) {
	hosts, err := s.networkService.ListHosts(req.Context())
	if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hosts) //nolint:errchkjson // ignore
	return http.StatusOK, nil
}
