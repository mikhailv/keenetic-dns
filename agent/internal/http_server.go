package internal

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/klauspost/compress/gzhttp"
	"github.com/klauspost/compress/gzip"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"

	"github.com/mikhailv/keenetic-dns/agent/internal/api"
	"github.com/mikhailv/keenetic-dns/internal/srv"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type HTTPServer struct {
	server         srv.HTTP
	networkService api.StrictServerInterface
}

func NewHTTPServer(addr string, logger *slog.Logger, networkService api.StrictServerInterface) *HTTPServer {
	return &HTTPServer{
		server:         srv.NewHTTPServer(addr, logger, nil),
		networkService: networkService,
	}
}

func (s *HTTPServer) Serve(ctx context.Context) error {
	s.server.Handler = s.createHandler()
	return s.server.Serve(ctx)
}

func (s *HTTPServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())

	apiServer := api.NewStrictHandler(s.networkService, nil)

	handler := cors.Default().Handler(api.HandlerFromMux(apiServer, mux))
	gzWrapper := util.UnwrapResult(gzhttp.NewWrapper(gzhttp.CompressionLevel(gzip.BestSpeed)))
	return gzWrapper(handler)
}
