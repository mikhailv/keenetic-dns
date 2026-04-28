package server

import (
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/klauspost/compress/gzhttp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/routing"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/srv"
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const dnsMessageMediaType = "application/dns-message"

type FilterFunc[T any] func(val T) bool

type HTTPServer struct {
	logger           *slog.Logger
	server           srv.HTTP
	resolver         dnssvc.Resolver
	ipRoutes         *routing.IPRouteController
	networkService   agentclient.NetworkServiceClient
	logStream        *stream.Buffered[log.Entry]
	queryStream      *stream.Buffered[types.DNSQuery]
	rawQueryStream   *stream.Buffered[types.DNSRawQuery]
	conntrackTracker *conntrack.Tracker
}

func NewHTTPServer(
	addr string,
	logger *slog.Logger,
	resolver dnssvc.Resolver,
	ipRoutes *routing.IPRouteController,
	networkService agentclient.NetworkServiceClient,
	logStream *stream.Buffered[log.Entry],
	queryStream *stream.Buffered[types.DNSQuery],
	rawQueryStream *stream.Buffered[types.DNSRawQuery],
	conntrackTracker *conntrack.Tracker,
) *HTTPServer {
	return &HTTPServer{
		logger:           logger,
		server:           srv.NewHTTPServer(addr, logger, nil),
		resolver:         resolver,
		ipRoutes:         ipRoutes,
		networkService:   networkService,
		logStream:        logStream,
		queryStream:      queryStream,
		rawQueryStream:   rawQueryStream,
		conntrackTracker: conntrackTracker,
	}
}

func (s *HTTPServer) Serve(ctx context.Context) error {
	s.server.Handler = s.createHandler()
	return s.server.Serve(ctx)
}

func (s *HTTPServer) createHandler() http.Handler {
	wsLogger := log.WithPrefix(s.logger, "ws")

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.Handle("GET /dns-query", s.wrapHandler(s.handleDNSQueryGET))
	mux.Handle("POST /dns-query", s.wrapHandler(s.handleDNSQueryPOST))
	mux.Handle("GET /api/routes", s.wrapHandler(s.handleListRoutes))
	mux.Handle("GET /api/hosts", s.wrapHandler(s.handleListHosts))
	mux.Handle("GET /api/logs", s.wrapHandler(createListHandler(s.logStream, s.filterLogs)))
	mux.Handle("GET /api/logs/ws", createStreamHandler(s.logStream, wsLogger, s.filterLogs))
	mux.Handle("GET /api/dns-queries", s.wrapHandler(createListHandler(s.queryStream, s.filterQueries)))
	mux.Handle("GET /api/dns-queries/ws", createStreamHandler(s.queryStream, wsLogger, s.filterQueries))
	mux.Handle("GET /api/dns-raw-queries", s.wrapHandler(createListHandler(s.rawQueryStream, s.filterRawQueries)))
	mux.Handle("GET /api/dns-raw-queries/ws", createStreamHandler(s.rawQueryStream, wsLogger, s.filterRawQueries))
	mux.Handle("GET /api/conntrack/buckets", s.wrapHandler(s.handleListConntrackBuckets))
	mux.Handle("GET /static/", webBuildDirectoryHandler())
	mux.Handle("GET /favicon.svg", webBuildFileHandler("favicon.svg"))
	mux.Handle("GET /", webBuildFileHandler("index.html"))

	var handler http.Handler = mux
	handler = cors.Default().Handler(handler)
	gzWrapper := util.UnwrapResult(gzhttp.NewWrapper(gzhttp.CompressionLevel(gzip.BestSpeed)))
	return gzWrapper(handler)
}

type errorHandler func(w http.ResponseWriter, req *http.Request) (statusCode int, err error)

func (s *HTTPServer) wrapHandler(handler errorHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path := r.Method, r.URL.Path
		operation := fmt.Sprintf("%s %s", method, path)
		defer metrics.TrackDuration(operation)()
		statusCode, err := handler(w, r)
		if err != nil {
			w.WriteHeader(statusCode)
			s.logger.Error(err.Error(), "method", method, "path", path, "statusCode", statusCode)
		}
		metrics.TrackStatus(operation, strconv.Itoa(statusCode))
	})
}
