package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
)

func TestHandleListHosts_UnavailableUntilLoaded(t *testing.T) {
	s := &HTTPServer{logger: slog.New(slog.DiscardHandler), networkService: &hostsClient{err: errors.New("agent unavailable")}}
	s.refreshHosts(t.Context())

	rec := httptest.NewRecorder()
	status, err := s.handleListHosts(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/hosts", nil))

	require.NoError(t, err, "a missing list is not a server error")
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, "30", rec.Header().Get("Retry-After"))
}

func TestHandleListHosts_ServesLoadedList(t *testing.T) {
	s := serverWithHosts(t, &hostsClient{hosts: []agentclient.HostInfo{{Ip: new("192.168.1.10")}}})

	rec := httptest.NewRecorder()
	status, err := s.handleListHosts(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/hosts", nil))

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "max-age=30, stale-if-error=300", rec.Header().Get("Cache-Control"))

	var hosts []agentclient.HostInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hosts))
	assert.Len(t, hosts, 1)
}

func TestHandleListHosts_ServesStaleListAfterFailedRefresh(t *testing.T) {
	client := &hostsClient{hosts: []agentclient.HostInfo{{Ip: new("192.168.1.10")}}}
	s := serverWithHosts(t, client)

	client.hosts, client.err = nil, errors.New("agent unavailable")
	s.refreshHosts(t.Context())

	rec := httptest.NewRecorder()
	status, err := s.handleListHosts(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/hosts", nil))

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	var hosts []agentclient.HostInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hosts))
	assert.Len(t, hosts, 1)
}
