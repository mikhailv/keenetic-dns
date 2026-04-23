package internal

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/klauspost/compress/gzhttp"
	"github.com/rs/cors"

	"github.com/mikhailv/keenetic-dns/internal/srv"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type HTTPServer struct {
	server   srv.HTTP
	resolver Resolver
}

func NewHTTPServer(addr string, logger *slog.Logger, resolver Resolver) *HTTPServer {
	return &HTTPServer{
		server:   srv.NewHTTPServer(addr, logger, nil),
		resolver: resolver,
	}
}

func (s *HTTPServer) Serve(ctx context.Context) error {
	s.server.Handler = s.createHandler()
	return s.server.Serve(ctx)
}

func (s *HTTPServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ip", s.handleClientIP)
	mux.HandleFunc("GET /ip/{ip}", s.handleIP)

	handler := cors.Default().Handler(mux)
	gzWrapper := util.UnwrapResult(gzhttp.NewWrapper(gzhttp.CompressionLevel(gzip.BestSpeed)))
	return gzWrapper(handler)
}

func (s *HTTPServer) handleClientIP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		host = fwd
	}
	s.lookupAndRespond(w, host)
}

func (s *HTTPServer) handleIP(w http.ResponseWriter, r *http.Request) {
	s.lookupAndRespond(w, r.PathValue("ip"))
}

func (s *HTTPServer) lookupAndRespond(w http.ResponseWriter, ipStr string) {
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

	var info IPInfo
	if err := s.resolver.Lookup(ip, &info); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	info.Debug.LookupTime = time.Since(st).Seconds()
	info.Debug.Resolver = s.resolver.Name()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info) //nolint:errchkjson // ignore
}
