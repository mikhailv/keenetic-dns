package ipinfo

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type fakeIPInfo struct {
	calls   atomic.Int32
	ips     atomic.Int32
	failing atomic.Bool
}

func (f *fakeIPInfo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	if f.failing.Load() {
		http.Error(w, "down", http.StatusBadGateway)
		return
	}
	var ips []string
	_ = json.NewDecoder(r.Body).Decode(&ips)
	f.ips.Add(int32(len(ips)))
	items := make([]map[string]any, len(ips))
	for i, ip := range ips {
		octets := strings.Split(ip, ".")
		items[i] = map[string]any{
			"ip": ip,
			"geo": map[string]any{
				"network": fmt.Sprintf("%s.%s.0.0/16", octets[0], octets[1]),
				"asn":     map[string]any{"number": 35807, "organization": "SkyNet Ltd."},
				"country": map[string]any{"iso_code": "RU"},
				"city":    map[string]any{"name": "St Petersburg"},
			},
		}
	}
	_ = json.NewEncoder(w).Encode(items) //nolint:errchkjson // test fake, a failed write fails the test anyway
}

func newTestResolver(t *testing.T, cfg Config) (*Resolver, *fakeIPInfo, *time.Time) {
	t.Helper()
	fake := &fakeIPInfo{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	cfg.URL = srv.URL
	cfg.SetDefaults()
	r := NewResolver(cfg, slog.New(slog.DiscardHandler))
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	return r, fake, &now
}

func ips(s ...string) []types.IPv4 {
	res := make([]types.IPv4, len(s))
	for i, v := range s {
		res[i] = types.MustParseIPv4(v)
	}
	return res
}

func TestResolver_CachesByNetwork(t *testing.T) {
	r, fake, _ := newTestResolver(t, Config{})

	res := r.Lookup(t.Context(), ips("94.19.127.130", "94.19.127.130"))
	require.Len(t, res, 1)
	assert.Equal(t, Info{Network: "94.19.0.0/16", ASN: 35807, Org: "SkyNet Ltd.", Country: "RU", City: "St Petersburg"},
		res[types.MustParseIPv4("94.19.127.130")])
	assert.Equal(t, int32(1), fake.ips.Load(), "duplicates are requested once")

	res = r.Lookup(t.Context(), ips("94.19.1.1"))
	assert.Equal(t, "94.19.0.0/16", res[types.MustParseIPv4("94.19.1.1")].Network)
	assert.Equal(t, int32(1), fake.calls.Load(), "an address inside a cached network needs no request")
}

func TestResolver_RefetchesStaleEntries(t *testing.T) {
	r, fake, now := newTestResolver(t, Config{TTL: time.Hour})

	r.Lookup(t.Context(), ips("94.19.127.130"))
	*now = now.Add(59 * time.Minute)
	r.Lookup(t.Context(), ips("94.19.127.130"))
	assert.Equal(t, int32(1), fake.calls.Load())

	*now = now.Add(2 * time.Minute)
	res := r.Lookup(t.Context(), ips("94.19.127.130"))
	assert.Len(t, res, 1)
	assert.Equal(t, int32(2), fake.calls.Load(), "a stale entry is fetched again")
}

func TestResolver_SkipsNonPublicAddresses(t *testing.T) {
	r, fake, _ := newTestResolver(t, Config{})

	res := r.Lookup(t.Context(), ips("192.168.2.91", "10.1.2.3", "100.106.0.1", "224.0.0.1", "0.0.0.0", "127.0.0.1"))
	assert.Empty(t, res)
	assert.Equal(t, int32(0), fake.calls.Load())
}

func TestResolver_FailsFastWhileUnavailable(t *testing.T) {
	r, fake, now := newTestResolver(t, Config{RetryAfter: time.Minute})
	r.Lookup(t.Context(), ips("94.19.127.130"))
	fake.failing.Store(true)

	res := r.Lookup(t.Context(), ips("94.19.127.130", "8.8.8.8"))
	assert.Len(t, res, 1, "cached entries are still served")
	assert.Equal(t, int32(2), fake.calls.Load())

	r.Lookup(t.Context(), ips("8.8.8.8"))
	assert.Equal(t, int32(2), fake.calls.Load(), "no requests within retry_after")

	fake.failing.Store(false)
	*now = now.Add(time.Minute)
	res = r.Lookup(t.Context(), ips("8.8.8.8"))
	assert.Len(t, res, 1)
	assert.Equal(t, int32(3), fake.calls.Load(), "requests resume after retry_after")
}

func TestResolver_SplitsLargeBatches(t *testing.T) {
	r, fake, _ := newTestResolver(t, Config{})
	var list []types.IPv4
	for i := range 2500 {
		list = append(list, types.MustParseIPv4(fmt.Sprintf("%d.%d.1.1", 20+i/200, i%200)))
	}

	res := r.Lookup(t.Context(), list)
	assert.Len(t, res, 2500)
	assert.Equal(t, int32(3), fake.calls.Load())
}

func TestResolver_DisabledWithoutURL(t *testing.T) {
	r := NewResolver(Config{}, slog.New(slog.DiscardHandler))
	assert.Empty(t, r.Lookup(t.Context(), ips("8.8.8.8")))
	var nilResolver *Resolver
	assert.Empty(t, nilResolver.Lookup(t.Context(), ips("8.8.8.8")))
}
