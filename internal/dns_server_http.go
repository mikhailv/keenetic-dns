package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	dnsMessageMediaType = "application/dns-message"
)

type HTTPServer struct {
	logger        *slog.Logger
	config        *Config
	resolver      DNSResolver
	server        http.Server
	ipRoutes      *IPRouteController
	logStream     *util.BufferedStream[log.Entry]
	resolveStream *util.BufferedStream[DomainResolve]
}

func NewHTTPServer(
	addr string,
	logger *slog.Logger,
	config *Config,
	resolver DNSResolver,
	ipRoutes *IPRouteController,
	logStream *util.BufferedStream[log.Entry],
	resolveStream *util.BufferedStream[DomainResolve],
) *HTTPServer {
	return &HTTPServer{
		logger:   logger,
		config:   config,
		resolver: resolver,
		server: http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
		},
		ipRoutes:      ipRoutes,
		logStream:     logStream,
		resolveStream: resolveStream,
	}
}

func (s *HTTPServer) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("http: failed to shutdown server", slog.Any("err", err))
		}
	}()
	s.server.Handler = s.createHandler()
	s.logger.Info("http: server starting...", slog.String("addr", s.server.Addr))
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("http: failed to start server", slog.Any("err", err))
	}
}

func (s *HTTPServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.Handle("POST /dns-query", s.wrapHandler(s.handleDNSQuery))
	mux.Handle("GET /routes", http.HandlerFunc(s.handleRoutes))
	mux.Handle("GET /logs", createStreamListHandler(s.logStream, s.filterLogs))
	mux.Handle("GET /dns-resolve", createStreamListHandler(s.resolveStream, s.filterResolves))
	return mux
}

func (s *HTTPServer) filterLogs(req *http.Request, query url.Values) func(val log.Entry) bool {
	levels := slices.DeleteFunc(strings.Split(query.Get("level"), ","), func(s string) bool { return s == "" })
	if len(levels) == 0 {
		return nil
	}
	levelSet := map[string]bool{}
	for _, level := range levels {
		levelSet[level] = true
	}
	return func(val log.Entry) bool {
		return levelSet[val.Level]
	}
}

func (s *HTTPServer) filterResolves(req *http.Request, query url.Values) func(val DomainResolve) bool {
	domain := query.Get("domain")
	if domain == "" {
		return nil
	}
	return func(val DomainResolve) bool {
		return val.Domain == domain
	}
}

func (s *HTTPServer) wrapHandler(handler func(w http.ResponseWriter, req *http.Request) (statusCode int, err error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path := r.Method, r.URL.Path
		operation := fmt.Sprintf("%s %s", method, path)
		defer TrackDuration(operation)()
		statusCode, err := handler(w, r)
		if err != nil {
			w.WriteHeader(statusCode)
			s.logger.Error(err.Error(), "method", method, "path", path, "statusCode", statusCode)
		}
		TrackStatus(operation, strconv.Itoa(statusCode))
	})
}

func (s *HTTPServer) handleDNSQuery(w http.ResponseWriter, req *http.Request) (statusCode int, err error) {
	if req.Header.Get("Content-Type") != dnsMessageMediaType || req.Header.Get("Accept") != dnsMessageMediaType {
		return http.StatusBadRequest, errors.New("http: unexpected request format")
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("http: failed to read body: %w", err)
	}

	var dnsReq dns.Msg
	if err = dnsReq.Unpack(body); err != nil {
		return http.StatusBadRequest, fmt.Errorf("http: failed to unpack DNS request: %w", err)
	}

	dnsResp, err := s.resolver.Resolve(req.Context(), &dnsReq)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("http: failed to send DNS request: %w", err)
	}

	dnsRespBytes, err := dnsResp.Pack()
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("http: failed to pack DNS response: %w", err)
	}

	w.Header().Set("Content-Type", dnsMessageMediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(dnsRespBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dnsRespBytes)

	return http.StatusOK, nil
}

func (s *HTTPServer) handleRoutes(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.ipRoutes.Routes()) //nolint:errchkjson // ignore any error
}

func createStreamListHandler[T any](stream *util.BufferedStream[T], filterFactory func(r *http.Request, q url.Values) func(val T) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query()
		filter := filterFactory(req, query)

		backward := queryParamSet(query, "backward")

		cursor := int64(0)
		if query.Has("cursor") {
			cursor, _ = strconv.ParseInt(query.Get("cursor"), 10, 64)
			cursor = max(0, cursor)
		} else if backward {
			cursor = math.MaxInt64
		}

		count := 50
		if query.Has("count") {
			count, _ = strconv.Atoi(query.Get("count"))
			count = max(1, count)
		}

		res := struct {
			Items    []T    `json:"items"`
			Cursor   uint64 `json:"cursor"`
			HasMore  bool   `json:"hasMore"`
			NextPage string `json:"nextPage,omitempty"`
		}{}

		if backward {
			res.Items, res.Cursor, res.HasMore = stream.QueryBackward(uint64(cursor), count, filter)
		} else {
			res.Items, res.Cursor, res.HasMore = stream.Query(uint64(cursor), count, filter)
		}
		if res.Items == nil {
			res.Items = []T{}
		}

		nextPageURL := updateURLQuery(*req.URL, map[string]string{"cursor": fmt.Sprint(res.Cursor)})
		res.NextPage = nextPageURL.String()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})
}

func queryParamSet(q url.Values, name string) bool {
	if q.Has(name) {
		v := q.Get(name)
		return v == "" || v == "1" || strings.ToLower(v) == "true"
	}
	return false
}

func updateURLQuery(u url.URL, values map[string]string) url.URL {
	q := u.Query()
	for k, v := range values {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u
}
