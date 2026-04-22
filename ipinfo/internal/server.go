package internal

import (
	"encoding/json"
	"net"
	"net/http"
	"time"
)

type Server struct {
	resolver Resolver
}

func NewServer(resolver Resolver) *Server {
	return &Server{resolver: resolver}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /ip", s.handleClientIP)
	mux.HandleFunc("GET /ip/{ip}", s.handleIP)
}

func (s *Server) handleClientIP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		host = fwd
	}
	s.lookupAndRespond(w, host)
}

func (s *Server) handleIP(w http.ResponseWriter, r *http.Request) {
	s.lookupAndRespond(w, r.PathValue("ip"))
}

func (s *Server) lookupAndRespond(w http.ResponseWriter, ipStr string) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		http.Error(w, "invalid IP address", http.StatusBadRequest)
		return
	}
	if ip.To4() == nil {
		http.Error(w, "only IPv4 is supported", http.StatusBadRequest)
		return
	}
	st := time.Now()
	info, err := s.resolver.Lookup(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	info.Debug.LookupTime = time.Since(st).Seconds()
	info.Debug.Resolver = s.resolver.Name()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}
