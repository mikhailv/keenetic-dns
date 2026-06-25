package internal

import (
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"iter"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/gzhttp"
	"github.com/rs/cors"

	"github.com/mikhailv/keenetic-dns/internal/srv"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const defaultQueryLimit = 100

//go:embed favicon.ico
var faviconICO []byte

type HTTPServer struct {
	server   srv.HTTP
	geo      *Dataset[GeoRecord, GeoRow]
	proxy    *Dataset[ProxyInfo, ProxyInfo]
	cacheTTL time.Duration
}

func NewHTTPServer(
	addr string,
	logger *slog.Logger,
	geo *Dataset[GeoRecord, GeoRow],
	proxy *Dataset[ProxyInfo, ProxyInfo],
	cacheTTL time.Duration,
) *HTTPServer {
	return &HTTPServer{
		server:   srv.NewHTTPServer(addr, logger, nil),
		geo:      geo,
		proxy:    proxy,
		cacheTTL: cacheTTL,
	}
}

func (s *HTTPServer) Serve(ctx context.Context) error {
	s.server.Handler = s.createHandler()
	return s.server.Serve(ctx)
}

func (s *HTTPServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	// root-level shortcuts that mirror /ip and /ip/{ip} for quick browser use (not part of public API).
	// {$} matches only the literal "/", so deep paths still 404 instead of falling through to handleClientIP.
	mux.HandleFunc("GET /{$}", s.handleClientIP)
	mux.HandleFunc("GET /{ip}", s.handleIP)
	mux.HandleFunc("GET /favicon.ico", s.handleFavicon)
	// API endpoints
	mux.HandleFunc("GET /ip", s.handleClientIP)
	mux.HandleFunc("GET /ip/{ip}", s.handleIP)
	mux.HandleFunc("GET /ip/query", s.handleGeoQuery)
	mux.HandleFunc("GET /proxy/query", s.handleProxyQuery)

	handler := cors.Default().Handler(mux)
	gzWrapper := util.UnwrapResult(gzhttp.NewWrapper(gzhttp.CompressionLevel(gzip.BestSpeed)))
	return gzWrapper(handler)
}

func (s *HTTPServer) handleFavicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	_, _ = w.Write(faviconICO)
}

func (s *HTTPServer) handleClientIP(w http.ResponseWriter, r *http.Request) {
	// The response depends on the caller's own address, so it must never be shared by a CDN.
	s.lookupIP(w, r.Context(), clientIP(r), false)
}

// clientIP resolves the originating client address, preferring proxy headers over the direct peer. Behind Cloudflare,
// CF-Connecting-IP holds the true visitor; X-Forwarded-For is the fallback (its first entry is the original client).
func clientIP(r *http.Request) string {
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return cf
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		return strings.TrimSpace(first)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *HTTPServer) handleIP(w http.ResponseWriter, r *http.Request) {
	// Keyed by the IP in the path, so the response is deterministic and safe to cache.
	s.lookupIP(w, r.Context(), r.PathValue("ip"), true)
}

func (s *HTTPServer) lookupIP(w http.ResponseWriter, ctx context.Context, ipStr string, cacheable bool) {
	ip, err := parseIPv4(ipStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var result IPLookup
	result.IP = ip.String()

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

	s.setCacheControl(w, cacheable)
	writeJSON(w, result)
}

// setCacheControl emits a Cache-Control header so an upstream CDN (e.g. Cloudflare) and browsers can cache
// deterministic responses for cacheTTL. Non-cacheable or TTL<=0 responses are marked no-store so nothing caches them.
func (s *HTTPServer) setCacheControl(w http.ResponseWriter, cacheable bool) {
	if cacheable && s.cacheTTL > 0 {
		maxAge := strconv.FormatInt(int64(s.cacheTTL/time.Second), 10)
		w.Header().Set("Cache-Control", "public, max-age="+maxAge)
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
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
	s.setCacheControl(w, true)
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
	s.setCacheControl(w, true)
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

func parseIPv4(s string) (net.IP, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, errors.New("invalid IP address")
	}
	if ip.To4() == nil {
		return nil, errors.New("only IPv4 is supported")
	}
	return ip, nil
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
