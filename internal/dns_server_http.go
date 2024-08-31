package internal

import (
	"bytes"
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

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	dnsMessageMediaType = "application/dns-message"
)

type FilterFunc[T any] func(val T) bool

type HTTPServer struct {
	logger        *slog.Logger
	resolver      DNSResolver
	server        http.Server
	ipRoutes      *IPRouteController
	logStream     *util.BufferedStream[log.Entry]
	resolveStream *util.BufferedStream[DomainResolve]
}

func NewHTTPServer(
	addr string,
	logger *slog.Logger,
	resolver DNSResolver,
	ipRoutes *IPRouteController,
	logStream *util.BufferedStream[log.Entry],
	resolveStream *util.BufferedStream[DomainResolve],
) *HTTPServer {
	return &HTTPServer{
		logger:   logger,
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
	s.server.Handler = s.createHandler()

	context.AfterFunc(ctx, func() {
		s.logger.Info("http: shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("http: failed to shutdown server", "err", err)
		}
	})

	s.logger.Info("http: server starting...", "addr", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("http: failed to start server", "err", err)
	}
}

func (s *HTTPServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.Handle("POST /dns-query", s.wrapHandler(s.handleDNSQuery))
	mux.Handle("GET /routes", http.HandlerFunc(s.handleRoutes))
	mux.Handle("GET /logs", createListHandler(s.logStream, s.filterLogs))
	mux.Handle("GET /logs/ws", createStreamHandler(s.logStream, s.logger, s.filterLogs))
	mux.Handle("GET /dns-resolve", createListHandler(s.resolveStream, s.filterResolves))
	mux.Handle("GET /dns-resolve/ws", createStreamHandler(s.resolveStream, s.logger, s.filterResolves))
	return mux
}

func (s *HTTPServer) filterLogs(_ *http.Request, query url.Values) FilterFunc[log.Entry] {
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

func (s *HTTPServer) filterResolves(_ *http.Request, query url.Values) FilterFunc[DomainResolve] {
	domain := strings.TrimSpace(query.Get("domain"))
	search := strings.TrimSpace(query.Get("search"))
	excludeRouted := queryParamSet(query, "exclude_routed")
	if domain == "" && search == "" && !excludeRouted {
		return nil
	}
	return func(val DomainResolve) bool {
		if excludeRouted && s.ipRoutes.LookupHost(val.Domain) != "" {
			return false
		}
		if search != "" {
			return strings.Contains(val.Domain, search)
		}
		if domain != "" {
			return val.Domain == domain
		}
		return true
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
	routes := s.ipRoutes.Routes()
	slices.SortFunc(routes, func(a, b IPRoute) int {
		return bytes.Compare(a.IP[:], b.IP[:])
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(routes) //nolint:errchkjson // ignore any error
}

type requestFilterFactory[T any] func(r *http.Request, q url.Values) FilterFunc[T]

func createListHandler[T any](stream *util.BufferedStream[T], filterFactory requestFilterFactory[T]) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query()
		filter := filterFactory(req, query)

		backward := queryParamSet(query, "backward")

		afterMode := true
		cursor := int64(0)
		if query.Has("after") {
			cursor, _ = strconv.ParseInt(query.Get("after"), 10, 64)
			cursor = max(0, cursor)
		} else if query.Has("before") {
			afterMode = false
			cursor, _ = strconv.ParseInt(query.Get("before"), 10, 64)
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
			util.QueryResult[T]
			PrevPageURL string `json:"prevPageURL"`
			NextPageURL string `json:"nextPageURL"`
		}{}

		if backward == afterMode {
			res.QueryResult = stream.QueryBackward(uint64(cursor), count, filter)
		} else {
			res.QueryResult = stream.Query(uint64(cursor), count, filter)
		}

		if backward == !afterMode {
			res.Reverse()
		}

		res.PrevPageURL = updateURLQuery(*req.URL, map[string]string{"after": "\x00", "before": fmt.Sprint(res.FirstCursor)})
		res.NextPageURL = updateURLQuery(*req.URL, map[string]string{"after": fmt.Sprint(res.LastCursor), "before": "\x00"})

		if res.Items == nil {
			res.Items = []T{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})
}

func createStreamHandler[T any](stream *util.BufferedStream[T], logger *slog.Logger, filterFactory requestFilterFactory[T]) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query()
		filter := filterFactory(req, query)

		conn, err := websocket.Accept(w, req, nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			logger.Error("http.ws: failed to accept websocket connection", "err", err)
			return
		}
		defer func() { _ = conn.CloseNow() }()

		logger.Debug("http.ws: accept websocket connection", "client", req.RemoteAddr)
		ctx := conn.CloseRead(req.Context())

		cursor := stream.QueryBackward(math.MaxUint64, 1, nil).FirstCursor // get last cursor from stream

		updateCh := make(chan struct{})
		throttledUpdateCh := throttleUpdateChannel(ctx, time.Second, updateCh)

		stopListen := stream.Listen(func(uint64, T) {
			select {
			case <-ctx.Done():
			case updateCh <- struct{}{}: // signal about update
			default: // do not block
			}
		})
		defer stopListen()

		for {
			select {
			case <-ctx.Done():
				logger.Debug("http.ws: websocket connection closed", "err", ctx.Err())
				return
			case <-throttledUpdateCh:
				res := stream.Query(cursor, 1000, filter)
				if len(res.Items) > 0 {
					if err := wsjson.Write(ctx, conn, res.Items); err != nil {
						logger.Error("http.ws: failed to send data", "err", err, "cursor", cursor)
					} else {
						cursor = res.LastCursor
					}
				}
			}
		}
	})
}

func throttleUpdateChannel(ctx context.Context, interval time.Duration, ch <-chan struct{}) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		var timer <-chan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ch:
				if !ok {
					return
				}
				if timer == nil {
					timer = time.After(interval)
				}
			case <-timer:
				timer = nil
				select {
				case <-ctx.Done():
				case out <- struct{}{}:
				}
			}
		}
	}()
	return out
}

func queryParamSet(q url.Values, name string) bool {
	if q.Has(name) {
		v := q.Get(name)
		return v == "" || v == "1" || strings.ToLower(v) == "true"
	}
	return false
}

func updateURLQuery(u url.URL, values map[string]string) string {
	q := u.Query()
	for k, v := range values {
		if v == "\x00" {
			q.Del(k)
		} else {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
