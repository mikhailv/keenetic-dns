package blocklist

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func listOf(domains ...string) string {
	var sb strings.Builder
	sb.WriteString("# generated\n")
	for _, d := range domains {
		sb.WriteString(d + "\n")
	}
	return sb.String()
}

var testClient = types.MustParseIPv4("192.168.1.10")

func newTestManager(t *testing.T, lists ...List) *Manager {
	t.Helper()
	m := NewManager(Config{
		Enabled:         true,
		DataDir:         t.TempDir(),
		Lists:           lists,
		RefreshInterval: time.Hour,
		DownloadTimeout: 5 * time.Second,
	}, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { assert.NoError(t, m.Close()) })
	return m
}

func blockList(name string, urls ...string) List {
	return List{Name: name, Enabled: true, URLs: urls}
}

func TestManager_RefreshBuildsIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	require.False(t, m.Loaded())
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, m.Loaded())

	match, ok := m.Lookup("tracker.ads.example.com", testClient)
	require.True(t, ok)
	assert.True(t, match.Blocked())
	assert.Equal(t, "ads", match.List)

	_, ok = m.Lookup("example.org", testClient)
	assert.False(t, ok)
}

func TestManager_StartRetriesUntilAnIndexIsBuilt(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	m.retryDelay, m.maxRetryDelay = time.Millisecond, 100*time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiter := m.Start(ctx)

	require.Eventually(t, m.Loaded, 5*time.Second, 5*time.Millisecond)
	assert.GreaterOrEqual(t, attempts.Load(), int32(3), "must have retried past the failing responses")

	match, ok := m.Lookup("ads.example.com", testClient)
	require.True(t, ok)
	assert.True(t, match.Blocked())

	cancel()
	waiter.Wait()
}

func TestManager_StartRetryStopsOnContextCancel(t *testing.T) {
	m := newTestManager(t, blockList("ads", "https://example.invalid/list.txt"))
	m.retryDelay, m.maxRetryDelay = time.Hour, time.Hour

	ctx, cancel := context.WithCancel(t.Context())
	waiter := m.Start(ctx)
	cancel()

	done := make(chan struct{})
	go func() {
		waiter.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not stop after the context was cancelled")
	}
}

func TestManager_LookupWithoutIndex(t *testing.T) {
	m := newTestManager(t)
	_, ok := m.Lookup("ads.example.com", testClient)
	assert.False(t, ok, "lookup must be inert when no index is loaded")
}

func TestManager_UnchangedListSkipsRebuild(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	require.NoError(t, m.Refresh(t.Context()))

	built, err := os.Stat(m.IndexPath())
	require.NoError(t, err)

	require.NoError(t, m.Refresh(t.Context()))
	after, err := os.Stat(m.IndexPath())
	require.NoError(t, err)

	assert.Equal(t, built.ModTime(), after.ModTime(), "unchanged lists must not rebuild the index")
	assert.Equal(t, int32(2), requests.Load())
}

func TestManager_ChangedListRebuilds(t *testing.T) {
	var body atomic.Value
	body.Store(listOf("ads.example.com"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current, _ := body.Load().(string)
		io.WriteString(w, current) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	require.NoError(t, m.Refresh(t.Context()))
	_, ok := m.Lookup("other.example.com", testClient)
	require.False(t, ok)

	body.Store(listOf("ads.example.com", "other.example.com"))
	require.NoError(t, m.Refresh(t.Context()))

	match, ok := m.Lookup("other.example.com", testClient)
	require.True(t, ok)
	assert.True(t, match.Blocked())
}

func TestManager_StartUsesCachedIndexWithoutRebuilding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := Config{
		Enabled:         true,
		DataDir:         dir,
		Lists:           []List{blockList("ads", srv.URL)},
		RefreshInterval: time.Hour,
		DownloadTimeout: 5 * time.Second,
	}

	first := NewManager(cfg, slog.New(slog.DiscardHandler))
	require.NoError(t, first.Refresh(t.Context()))
	built, err := os.Stat(first.IndexPath())
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second := NewManager(cfg, slog.New(slog.DiscardHandler))
	defer second.Close()
	require.NoError(t, second.loadCached())
	assert.True(t, second.Loaded())

	after, err := os.Stat(second.IndexPath())
	require.NoError(t, err)
	assert.Equal(t, built.ModTime(), after.ModTime())

	match, ok := second.Lookup("x.ads.example.com", testClient)
	require.True(t, ok)
	assert.True(t, match.Blocked())
}

func TestManager_StaleCachedIndexRejected(t *testing.T) {
	m := newTestManager(t, blockList("ads", "https://example.invalid/list.txt"))

	b := NewBuilder()
	id, err := b.AddList("ads", "")
	require.NoError(t, err)
	b.Add(Rule{Domain: "ads.example.com", Action: Block, Subdomains: true}, id)
	require.NoError(t, b.Write(m.IndexPath(), time.Now()))

	require.Error(t, m.loadCached())
	assert.False(t, m.Loaded())
}

func TestManager_FailedRefreshKeepsPreviousIndex(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	require.NoError(t, m.Refresh(t.Context()))

	fail.Store(true)
	require.NoError(t, m.Refresh(t.Context()), "cached list keeps a failed refresh non-fatal")

	match, ok := m.Lookup("ads.example.com", testClient)
	require.True(t, ok, "previous index must stay live after a failed refresh")
	assert.True(t, match.Blocked())
}

func TestManager_RefreshWithNoSources(t *testing.T) {
	m := newTestManager(t)
	require.NoError(t, m.Refresh(t.Context()))
	assert.False(t, m.Loaded())
}

func TestManager_MultipleLists(t *testing.T) {
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer ads.Close()
	malware := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("evil.example.net")) //nolint:errcheck // test
	}))
	defer malware.Close()

	m := newTestManager(t,
		blockList("ads", ads.URL),
		blockList("malware", malware.URL),
	)
	require.NoError(t, m.Refresh(t.Context()))

	match, ok := m.Lookup("ads.example.com", testClient)
	require.True(t, ok)
	assert.Equal(t, "ads", match.List)

	match, ok = m.Lookup("evil.example.net", testClient)
	require.True(t, ok)
	assert.Equal(t, "malware", match.List)
}

func TestManager_ConcurrentLookupDuringRebuild(t *testing.T) {
	var counter atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := counter.Add(1)
		io.WriteString(w, listOf("ads.example.com", fmt.Sprintf("v%d.example.org", n))) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	require.NoError(t, m.Refresh(t.Context()))

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 8 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if match, ok := m.Lookup("tracker.ads.example.com", testClient); ok {
					assert.True(t, match.Blocked())
				}
			}
		})
	}

	for range 15 {
		require.NoError(t, m.Refresh(t.Context()))
	}
	close(stop)
	wg.Wait()

	match, ok := m.Lookup("ads.example.com", testClient)
	require.True(t, ok)
	assert.True(t, match.Blocked())
}

func TestManager_IndexPath(t *testing.T) {
	m := NewManager(Config{DataDir: "/var/lib/blocklist"}, slog.New(slog.DiscardHandler))
	assert.Equal(t, filepath.Join("/var/lib/blocklist", IndexFileName), m.IndexPath())
}

func TestManager_PerClientGroups(t *testing.T) {
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer ads.Close()
	strict := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("gaming.example.net")) //nolint:errcheck // test
	}))
	defer strict.Close()

	m := NewManager(Config{
		Enabled: true,
		DataDir: t.TempDir(),
		Lists: []List{
			blockList("ads", ads.URL),
			blockList("strict", strict.URL),
		},
		RefreshInterval: time.Hour,
		DownloadTimeout: 5 * time.Second,
		Groups: []Group{
			{Name: "kids", Clients: []string{"192.168.1.50", "192.168.9.0/24"}, Lists: []string{"ads", "strict"}},
			{Name: "relaxed", Clients: []string{"192.168.1.51"}, Lists: []string{"ads"}},
		},
	}, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { assert.NoError(t, m.Close()) })
	require.NoError(t, m.Refresh(t.Context()))

	kid := types.MustParseIPv4("192.168.1.50")
	inKidSubnet := types.MustParseIPv4("192.168.9.77")
	relaxed := types.MustParseIPv4("192.168.1.51")
	ungrouped := types.MustParseIPv4("10.0.0.5")

	for _, client := range []types.IPv4{kid, inKidSubnet} {
		match, ok := m.Lookup("gaming.example.net", client)
		require.True(t, ok, client.String())
		assert.True(t, match.Blocked(), client.String())
	}
	_, relaxedBlocked := m.Lookup("gaming.example.net", relaxed)
	assert.False(t, relaxedBlocked, "a list outside the client's group must not apply")

	for _, client := range []types.IPv4{kid, relaxed, ungrouped} {
		match, ok := m.Lookup("ads.example.com", client)
		require.True(t, ok, client.String())
		assert.True(t, match.Blocked(), client.String())
	}

	match, ok := m.Lookup("gaming.example.net", ungrouped)
	require.True(t, ok)
	assert.True(t, match.Blocked())
}

func TestManager_AllowURLsAndHandWrittenRules(t *testing.T) {
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("example.com", "keepme.example.org")) //nolint:errcheck // test
	}))
	defer ads.Close()
	rescue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("shop.example.com")) //nolint:errcheck // test
	}))
	defer rescue.Close()

	m := NewManager(Config{
		Enabled: true,
		DataDir: t.TempDir(),
		Lists: []List{{
			Name:      "ads",
			Enabled:   true,
			URLs:      []string{ads.URL},
			AllowURLs: []string{rescue.URL},
			Allow:     []string{"keepme.example.org"},
			Deny:      []string{"nope.example.net"},
		}},
		RefreshInterval: time.Hour,
		DownloadTimeout: 5 * time.Second,
	}, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { assert.NoError(t, m.Close()) })
	require.NoError(t, m.Refresh(t.Context()))

	match, ok := m.Lookup("shop.example.com", testClient)
	require.True(t, ok)
	assert.False(t, match.Blocked())

	match, ok = m.Lookup("keepme.example.org", testClient)
	require.True(t, ok)
	assert.False(t, match.Blocked())

	match, ok = m.Lookup("sub.nope.example.net", testClient)
	require.True(t, ok)
	assert.True(t, match.Blocked())

	assert.Equal(t, "ads", match.List)

	_, ok = m.Lookup("unrelated.example.io", testClient)
	assert.False(t, ok)
}

func TestManager_ListWithoutURLs(t *testing.T) {
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer ads.Close()

	m := newTestManager(t,
		blockList("ads", ads.URL),
		List{Name: "overrides", Enabled: true, Allow: []string{"ads.example.com"}, Deny: []string{"mine.example.net"}},
	)
	require.NoError(t, m.Refresh(t.Context()))

	match, ok := m.Lookup("ads.example.com", testClient)
	require.True(t, ok)
	assert.False(t, match.Blocked(), "the overrides list rescues it")
	assert.Equal(t, "overrides", match.List)

	match, ok = m.Lookup("mine.example.net", testClient)
	require.True(t, ok)
	assert.True(t, match.Blocked())
	assert.Equal(t, "overrides", match.List)
}

func TestManager_MergesURLsIntoOneList(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("tracker.example.net")) //nolint:errcheck // test
	}))
	defer second.Close()

	m := newTestManager(t, blockList("ads", first.URL, second.URL))
	require.NoError(t, m.Refresh(t.Context()))

	for _, domain := range []string{"ads.example.com", "tracker.example.net"} {
		match, ok := m.Lookup(domain, testClient)
		require.True(t, ok, domain)
		assert.True(t, match.Blocked(), domain)
		assert.Equal(t, "ads", match.List, "merged urls report the one list name")
	}
}

func TestManager_LostURLKeepsContributingFromCache(t *testing.T) {
	var fail atomic.Bool
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer flaky.Close()
	steady := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("tracker.example.net")) //nolint:errcheck // test
	}))
	defer steady.Close()

	m := newTestManager(t, blockList("ads", flaky.URL, steady.URL))
	require.NoError(t, m.Refresh(t.Context()))

	fail.Store(true)
	require.NoError(t, m.Refresh(t.Context()))

	for _, domain := range []string{"ads.example.com", "tracker.example.net"} {
		match, ok := m.Lookup(domain, testClient)
		require.True(t, ok, domain)
		assert.True(t, match.Blocked(), domain)
	}
}

func TestManager_ConfigChangeRebuildsIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := func(allow ...string) Config {
		list := blockList("ads", srv.URL)
		list.Allow = allow
		return Config{
			Enabled:         true,
			DataDir:         dir,
			Lists:           []List{list},
			RefreshInterval: time.Hour,
			DownloadTimeout: 5 * time.Second,
		}
	}

	first := NewManager(cfg(), slog.New(slog.DiscardHandler))
	require.NoError(t, first.Refresh(t.Context()))
	match, ok := first.Lookup("ads.example.com", testClient)
	require.True(t, ok)
	require.True(t, match.Blocked())
	require.NoError(t, first.Close())

	second := NewManager(cfg("ads.example.com"), slog.New(slog.DiscardHandler))
	defer second.Close()

	require.Error(t, second.loadCached(), "a cached index built from another configuration must be rejected")
	assert.False(t, second.Loaded())

	require.NoError(t, second.Refresh(t.Context()))
	match, ok = second.Lookup("ads.example.com", testClient)
	require.True(t, ok)
	assert.False(t, match.Blocked(), "the rebuilt index must honour the new allow entry")
}

func TestManager_ConfigReorderKeepsCachedIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	dir := t.TempDir()
	build := func(deny ...string) Config {
		a := blockList("a-list", srv.URL)
		a.Deny = deny
		b := blockList("b-list", srv.URL+"/other")
		return Config{
			Enabled:         true,
			DataDir:         dir,
			Lists:           []List{a, b},
			RefreshInterval: time.Hour,
			DownloadTimeout: 5 * time.Second,
		}
	}

	first := NewManager(build("x.example.net", "y.example.net"), slog.New(slog.DiscardHandler))
	require.NoError(t, first.Refresh(t.Context()))
	built, err := os.Stat(first.IndexPath())
	require.NoError(t, err)
	require.NoError(t, first.Close())

	reordered := build("Y.EXAMPLE.NET", "x.example.net")
	reordered.Lists[0], reordered.Lists[1] = reordered.Lists[1], reordered.Lists[0]

	second := NewManager(reordered, slog.New(slog.DiscardHandler))
	defer second.Close()
	require.NoError(t, second.loadCached(), "a reordering is not a change")
	assert.True(t, second.Loaded())

	after, err := os.Stat(second.IndexPath())
	require.NoError(t, err)
	assert.Equal(t, built.ModTime(), after.ModTime(), "the index must not have been rebuilt")
}

func TestManager_StartRetriesWhileServingStaleCopy(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n := attempts.Add(1); n > 1 && n < 4 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("ETag", fmt.Sprintf(`"v%d"`, attempts.Load()))
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, m.Loaded(), "a cached copy must exist before the failures start")

	m.retryDelay, m.maxRetryDelay = time.Millisecond, 100*time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiter := m.Start(ctx)

	require.Eventually(t, func() bool { return attempts.Load() >= 4 }, 5*time.Second, 5*time.Millisecond,
		"must keep retrying past the failures instead of settling for the cached copy")
	assert.Eventually(t, func() bool { return !m.Degraded() }, time.Second, 5*time.Millisecond,
		"once a source answers again the lists are current")

	cancel()
	waiter.Wait()
}

func TestManager_StartStopsRetryingAtBackoffCeiling(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	require.NoError(t, m.Refresh(t.Context()))

	m.retryDelay, m.maxRetryDelay = time.Millisecond, 4*time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan struct{})
	go func() { m.startupRefresh(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("startupRefresh never gave up")
	}
	assert.True(t, m.Degraded(), "it gave up while still degraded")
	assert.LessOrEqual(t, attempts.Load(), int32(6), "retries must be bounded, not endless")
}

func TestManager_StartWaiterCoversStartupRefresh(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		io.WriteString(w, listOf("ads.example.com")) //nolint:errcheck // test
	}))
	defer srv.Close()

	m := newTestManager(t, blockList("ads", srv.URL))
	ctx, cancel := context.WithCancel(t.Context())
	waiter := m.Start(ctx)

	var done atomic.Bool
	go func() {
		waiter.Wait()
		done.Store(true)
	}()

	time.Sleep(50 * time.Millisecond)
	require.False(t, done.Load(), "waiter must not return while the startup refresh is in flight")

	cancel()
	close(release)
	require.Eventually(t, done.Load, 5*time.Second, 5*time.Millisecond)
}

func denyList(name string, domains ...string) List {
	return List{Name: name, Enabled: true, Deny: domains}
}

func newTestManagerCfg(t *testing.T, cfg Config) *Manager {
	t.Helper()
	cfg.Enabled = true
	cfg.DataDir = t.TempDir()
	cfg.RefreshInterval = time.Hour
	cfg.DownloadTimeout = 5 * time.Second
	m := NewManager(cfg, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { assert.NoError(t, m.Close()) })
	return m
}

func TestManager_UpdateConfigAppliesGroupsWithoutRebuilding(t *testing.T) {
	cfg := Config{
		Lists: []List{
			denyList("ads", "ads.example.com"),
			denyList("strict", "gaming.example.net"),
		},
		Groups: []Group{{Name: "relaxed", Clients: []string{"192.168.1.51"}, Lists: []string{"ads"}}},
	}
	m := newTestManagerCfg(t, cfg)
	require.NoError(t, m.Refresh(t.Context()))

	client := types.MustParseIPv4("192.168.1.51")
	_, ok := m.Lookup("gaming.example.net", client)
	require.False(t, ok)

	built, err := os.Stat(m.IndexPath())
	require.NoError(t, err)

	updated := cfg
	updated.Groups = []Group{{Name: "relaxed", Clients: []string{"192.168.1.51"}, Lists: []string{"ads", "strict"}}}
	m.UpdateConfig(updated)

	match, ok := m.Lookup("gaming.example.net", client)
	require.True(t, ok, "a group change must apply without waiting for a refresh")
	assert.True(t, match.Blocked())

	rebuilt, err := os.Stat(m.IndexPath())
	require.NoError(t, err)
	assert.Equal(t, built.ModTime(), rebuilt.ModTime(), "a group change must not rebuild the index")
	assert.Empty(t, m.reload, "a group change must not ask for a refresh")
}

func TestManager_UpdateConfigRefreshesOnListChange(t *testing.T) {
	m := newTestManagerCfg(t, Config{Lists: []List{denyList("custom", "ads.example.com")}})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiter := m.Start(ctx)
	require.Eventually(t, m.Loaded, 5*time.Second, 5*time.Millisecond)

	m.UpdateConfig(Config{Lists: []List{denyList("custom", "ads.example.com", "tracker.example.net")}})

	require.Eventually(t, func() bool {
		_, ok := m.Lookup("tracker.example.net", testClient)
		return ok
	}, 5*time.Second, 5*time.Millisecond, "a list change must rebuild the index")

	match, ok := m.Lookup("ads.example.com", testClient)
	require.True(t, ok)
	assert.True(t, match.Blocked(), "the unchanged rules must survive the rebuild")

	cancel()
	waiter.Wait()
}

func TestManager_UpdateConfigIgnoresUnchangedListsAndGroups(t *testing.T) {
	cfg := Config{
		Lists:  []List{denyList("ads", "ADS.example.com ")},
		Groups: []Group{{Name: "relaxed", Clients: []string{"192.168.1.51"}, Lists: []string{"ads"}}},
	}
	m := newTestManagerCfg(t, cfg)
	require.NoError(t, m.Refresh(t.Context()))

	m.UpdateConfig(cfg)
	assert.Empty(t, m.reload, "an unchanged config must not ask for a refresh")
}

func TestManager_UpdateConfigKeepsFixedSettings(t *testing.T) {
	m := newTestManagerCfg(t, Config{Lists: []List{denyList("ads", "ads.example.com")}})
	indexPath := m.IndexPath()

	m.UpdateConfig(Config{
		DataDir:         t.TempDir(),
		RefreshInterval: time.Minute,
		Lists:           []List{denyList("ads", "ads.example.com")},
	})

	assert.Equal(t, indexPath, m.IndexPath(), "data_dir must keep the value it had at startup")
	assert.Equal(t, time.Hour, m.config().RefreshInterval)
}

func TestManager_UpdateConfigAppliesMode(t *testing.T) {
	cfg := Config{Mode: ModeNXDomain, Lists: []List{denyList("ads", "ads.example.com")}}
	m := newTestManagerCfg(t, cfg)
	require.NoError(t, m.Refresh(t.Context()))
	require.Equal(t, ModeNXDomain, m.Mode())

	updated := cfg
	updated.Mode = ModeNull
	m.UpdateConfig(updated)

	assert.Equal(t, ModeNull, m.Mode())
	assert.Empty(t, m.reload, "a mode change must not ask for a refresh")
}

func TestManager_UpdateConfigDropsIndexWhenNoListsRemain(t *testing.T) {
	m := newTestManagerCfg(t, Config{Lists: []List{denyList("ads", "ads.example.com")}})
	require.NoError(t, m.Refresh(t.Context()))
	_, ok := m.Lookup("ads.example.com", testClient)
	require.True(t, ok)

	m.UpdateConfig(Config{})

	assert.False(t, m.Loaded(), "a config left without lists must drop the index")
	_, ok = m.Lookup("ads.example.com", testClient)
	assert.False(t, ok, "nothing may be blocked once the lists are gone")
	assert.Empty(t, m.reload, "an empty list set has nothing to download")
}

func TestManager_RefreshDropsIndexWhenNoListsRemain(t *testing.T) {
	m := newTestManagerCfg(t, Config{Lists: []List{denyList("ads", "ads.example.com")}})
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, m.Loaded())

	m.cfg.Store(newActiveConfig(Config{DataDir: m.config().DataDir}))
	require.NoError(t, m.Refresh(t.Context()))

	assert.False(t, m.Loaded())
}
