package server

import (
	"encoding/json"
	"net/http"

	agentv1 "github.com/mikhailv/keenetic-dns/agent/rpc/v1"
)

func (s *HTTPServer) handleListClients(w http.ResponseWriter, req *http.Request) (int, error) {
	clients, err := s.networkService.ListHosts(req.Context(), &agentv1.ListHostsReq{})
	if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(clients) //nolint:errchkjson // ignore any error
	return http.StatusOK, nil
}
