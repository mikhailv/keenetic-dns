package internal

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"iter"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/klauspost/compress/gzhttp"
	"github.com/rs/cors"

	"github.com/mikhailv/keenetic-dns/internal/srv"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const defaultQueryLimit = 100

type HTTPServer struct {
	server srv.HTTP
	geo    *Dataset[GeoRecord, GeoRow]
	proxy  *Dataset[ProxyInfo, ProxyInfo]
}

func NewHTTPServer(
	addr string,
	logger *slog.Logger,
	geo *Dataset[GeoRecord, GeoRow],
	proxy *Dataset[ProxyInfo, ProxyInfo],
) *HTTPServer {
	return &HTTPServer{
		server: srv.NewHTTPServer(addr, logger, nil),
		geo:    geo,
		proxy:  proxy,
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
	mux.HandleFunc("GET /ip/query", s.handleGeoQuery)
	mux.HandleFunc("GET /proxy/query", s.handleProxyQuery)

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
	s.lookupIP(w, r.Context(), host)
}

func (s *HTTPServer) handleIP(w http.ResponseWriter, r *http.Request) {
	s.lookupIP(w, r.Context(), r.PathValue("ip"))
}

func (s *HTTPServer) lookupIP(w http.ResponseWriter, ctx context.Context, ipStr string) {
	ip, ok := parseIPv4(w, ipStr)
	if !ok {
		return
	}

	var result IPLookup

	var rec GeoRecord
	geoNetwork, err := s.geo.Lookup(ctx, ip, &rec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if geoNetwork != "" {
		geo := rec.ToIPInfo()
		geo.Network = geoNetwork
		result.Geo = &geo
	}

	var proxy ProxyInfo
	proxyNetwork, err := s.proxy.Lookup(ctx, ip, &proxy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if proxyNetwork != "" {
		proxy.Network = proxyNetwork
		result.Proxy = &proxy
	}

	writeJSON(w, result)
}

func (s *HTTPServer) handleGeoQuery(w http.ResponseWriter, r *http.Request) {
	field, query, offset, limit, ok := parseQueryArgs(w, r)
	if !ok {
		return
	}
	resp, err := collectQuery(s.geo.Query(r.Context(), field, query), offset, limit, GeoRow.ToIPInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *HTTPServer) handleProxyQuery(w http.ResponseWriter, r *http.Request) {
	field, query, offset, limit, ok := parseQueryArgs(w, r)
	if !ok {
		return
	}
	resp, err := collectQuery(s.proxy.Query(r.Context(), field, query), offset, limit, identity[ProxyInfo])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

// collectQuery drains seq, skipping the first offset rows and accumulating up to limit converted rows. HasMore is true
// if seq had at least one more row after the limit was reached.
func collectQuery[P, R any](seq iter.Seq2[P, error], offset, limit int, convert func(P) R) (QueryResponse[R], error) {
	resp := QueryResponse[R]{Items: make([]R, 0, limit)}
	skipped := 0
	for row, err := range seq {
		if err != nil {
			return resp, err
		}
		if skipped < offset {
			skipped++
			continue
		}
		if len(resp.Items) >= limit {
			resp.HasMore = true
			break
		}
		resp.Items = append(resp.Items, convert(row))
	}
	return resp, nil
}

func identity[T any](v T) T { return v }

func parseIPv4(w http.ResponseWriter, ipStr string) (net.IP, bool) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		http.Error(w, "invalid IP address", http.StatusBadRequest)
		return nil, false
	}
	if ip.To4() == nil {
		http.Error(w, "only IPv4 is supported", http.StatusBadRequest)
		return nil, false
	}
	return ip, true
}

func parseQueryArgs(w http.ResponseWriter, r *http.Request) (field, query string, offset, limit int, ok bool) {
	q := r.URL.Query()
	field = q.Get("field")
	query = q.Get("query")
	if field == "" || query == "" {
		http.Error(w, "field and query parameters are required", http.StatusBadRequest)
		return "", "", 0, 0, false
	}
	limit = defaultQueryLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return "", "", 0, 0, false
		}
		limit = n
	}
	limit = max(1, limit)
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			http.Error(w, "offset must be a non-negative integer", http.StatusBadRequest)
			return "", "", 0, 0, false
		}
		offset = n
	}
	offset = max(0, offset)
	return field, query, offset, limit, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v) //nolint:errchkjson // ignore
}
