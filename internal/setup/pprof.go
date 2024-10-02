package setup

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"runtime"
	pp "runtime/pprof"
	"time"
)

func Pprof(ctx context.Context, addr string, logger *slog.Logger) {
	if addr == "" {
		return
	}

	runtime.MemProfileRate = 1

	var mux http.ServeMux
	mux.HandleFunc("/", pprof.Index)
	mux.HandleFunc("/cmdline", pprof.Cmdline)
	mux.HandleFunc("/profile", pprof.Profile)
	mux.HandleFunc("/symbol", pprof.Symbol)
	mux.HandleFunc("/trace", pprof.Trace)
	for _, p := range pp.Profiles() {
		name := p.Name()
		mux.HandleFunc("/"+name, func(rw http.ResponseWriter, req *http.Request) {
			req.URL.Path = "/debug/pprof/" + name
			pprof.Index(rw, req)
		})
	}

	srv := http.Server{
		Addr:              addr,
		Handler:           &mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("pprof handler started", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("failed to serve pprof handler", "err", err)
		}
	}()

	context.AfterFunc(ctx, func() {
		_ = srv.Close()
		logger.Info("pprof handler stopped")
	})
}
