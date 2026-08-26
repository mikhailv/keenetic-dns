package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	hostRefreshInterval = 30 * time.Second
	hostStaleIfError    = 5 * time.Minute
)

var errHostsNotLoaded = errors.New("host list not loaded yet")

func (s *HTTPServer) startHostRefresh(ctx context.Context) util.Waiter {
	waiter, stop := util.NewWaiter()
	go func() {
		defer stop()
		s.refreshHosts(ctx)
		util.RunPeriodically(ctx.Done(), hostRefreshInterval, func() {
			s.refreshHosts(ctx)
		}).Wait()
	}()
	return waiter
}

func (s *HTTPServer) refreshHosts(ctx context.Context) {
	hosts, err := s.networkService.ListHosts(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("failed to load host list", "err", err)
		}
		return
	}
	if hosts == nil {
		hosts = []agentclient.HostInfo{}
	}
	s.hosts.Store(&hosts)
}

func (s *HTTPServer) hostList() ([]agentclient.HostInfo, error) {
	hosts := s.hosts.Load()
	if hosts == nil {
		return nil, errHostsNotLoaded
	}
	return *hosts, nil
}

func (s *HTTPServer) handleListHosts(w http.ResponseWriter, req *http.Request) (int, error) {
	hosts, err := s.hostList()
	if errors.Is(err, errHostsNotLoaded) {
		s.logger.Debug("host list requested before it was loaded")
		w.Header().Set("Retry-After", strconv.Itoa(int(hostRefreshInterval.Seconds())))
		w.WriteHeader(http.StatusServiceUnavailable)
		return http.StatusServiceUnavailable, nil
	}
	if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d, stale-if-error=%d", int(hostRefreshInterval.Seconds()), int(hostStaleIfError.Seconds())))
	_ = json.NewEncoder(w).Encode(hosts) //nolint:errchkjson // ignore
	return http.StatusOK, nil
}
