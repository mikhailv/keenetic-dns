package blocklist

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testETag = `"v1"`

func sampleList() string {
	var sb strings.Builder
	sb.WriteString("# Test list\n\n")
	for i := range 50 {
		fmt.Fprintf(&sb, "ads%d.example.com\n", i)
	}
	return sb.String()
}

func newTestDownloader(t *testing.T) *Downloader {
	t.Helper()
	return NewDownloader(t.TempDir(), 5*time.Second, slog.New(slog.DiscardHandler))
}

func TestDownloader_FetchesAndCaches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer srv.Close()

	d := newTestDownloader(t)
	status, paths, err := d.Fetch(t.Context(), "test", []string{srv.URL})
	require.NoError(t, err)
	assert.True(t, status.Changed)
	require.Len(t, paths, 1)

	data, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), "ads0.example.com")

	meta, err := d.loadMeta(d.partMetaPath(srv.URL))
	require.NoError(t, err)
	assert.Equal(t, srv.URL, meta.URL)
	assert.Equal(t, 50, meta.Rules)
}

func TestDownloader_ConditionalRequestSkipsUnchanged(t *testing.T) {
	var requests, conditional int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("If-None-Match") == testETag {
			conditional++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", testETag)
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer srv.Close()

	d := newTestDownloader(t)

	status, _, err := d.Fetch(t.Context(), "test", []string{srv.URL})
	require.NoError(t, err)
	assert.True(t, status.Changed)

	status, _, err = d.Fetch(t.Context(), "test", []string{srv.URL})
	require.NoError(t, err)
	assert.False(t, status.Changed, "unchanged list must not be reported as changed")
	assert.Equal(t, 2, requests)
	assert.Equal(t, 1, conditional, "second request must be conditional")
}

func TestDownloader_MergesSeveralURLs(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.ReplaceAll(sampleList(), "ads", "trackers")) //nolint:errcheck // test
	}))
	defer second.Close()

	d := newTestDownloader(t)
	status, paths, err := d.Fetch(t.Context(), "test", []string{first.URL, second.URL})
	require.NoError(t, err)
	assert.True(t, status.Changed)
	require.Len(t, paths, 2, "every url contributes its own cached file")

	firstData, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	assert.Contains(t, string(firstData), "ads0.example.com")

	secondData, err := os.ReadFile(paths[1])
	require.NoError(t, err)
	assert.Contains(t, string(secondData), "trackers0.example.com")
}

func TestDownloader_FailedURLKeepsItsCachedCopy(t *testing.T) {
	var fail atomic.Bool
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer flaky.Close()
	steady := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.ReplaceAll(sampleList(), "ads", "trackers")) //nolint:errcheck // test
	}))
	defer steady.Close()

	d := newTestDownloader(t)
	urls := []string{flaky.URL, steady.URL}
	_, paths, err := d.Fetch(t.Context(), "test", urls)
	require.NoError(t, err)
	require.Len(t, paths, 2)

	fail.Store(true)
	_, paths, err = d.Fetch(t.Context(), "test", urls)
	require.NoError(t, err, "a failed url with a cached copy is not fatal")
	require.Len(t, paths, 2, "the failed url still contributes its cached copy")
}

func TestDownloader_NeverDownloadedURLIsSkipped(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer failing.Close()
	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer working.Close()

	d := newTestDownloader(t)
	_, paths, err := d.Fetch(t.Context(), "test", []string{failing.URL, working.URL})
	require.NoError(t, err)
	require.Len(t, paths, 1, "only the url that downloaded contributes")
}

func TestDownloader_ChangedURLIgnoresStaleValidators(t *testing.T) {
	var conditional atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			conditional.Add(1)
		}
		w.Header().Set("ETag", testETag)
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer srv.Close()

	d := newTestDownloader(t)
	_, _, err := d.Fetch(t.Context(), "test", []string{srv.URL})
	require.NoError(t, err)

	_, _, err = d.Fetch(t.Context(), "test", []string{srv.URL + "/other"})
	require.NoError(t, err)
	assert.Zero(t, conditional.Load(), "a changed url must be fetched unconditionally")
}

func TestDownloader_RejectsHTMLErrorPage(t *testing.T) {
	const page = `<!DOCTYPE html>
<html><head><title>Not Found</title></head>
<body><script>
root.classList.add('gl-dark');
window.matchMedia.addEventListener('change', e => root.classList.remove('gl-dark'));
</script></body></html>`

	tests := []struct {
		name        string
		contentType string
	}{
		{name: "html content type", contentType: "text/html; charset=utf-8"},
		{name: "mislabelled as text", contentType: "text/plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				io.WriteString(w, page) //nolint:errcheck // test
			}))
			defer srv.Close()

			d := newTestDownloader(t)
			_, _, err := d.Fetch(t.Context(), "test", []string{srv.URL})
			require.Error(t, err)
			assert.NoFileExists(t, d.partPath(srv.URL), "rejected body must not become the cached list")
		})
	}
}

func TestDownloader_RejectsLowYieldBody(t *testing.T) {
	var sb strings.Builder
	for range 200 {
		sb.WriteString("this line is not a domain rule at all\n")
	}
	sb.WriteString("ads.example.com\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, sb.String()) //nolint:errcheck // test
	}))
	defer srv.Close()

	d := newTestDownloader(t)
	_, _, err := d.Fetch(t.Context(), "test", []string{srv.URL})
	require.Error(t, err)
	assert.NoFileExists(t, d.partPath(srv.URL))
}

func TestDownloader_KeepsCachedCopyWhenAllSourcesFail(t *testing.T) {
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer srv.Close()

	d := newTestDownloader(t)
	urls := []string{srv.URL}
	_, _, err := d.Fetch(t.Context(), "test", urls)
	require.NoError(t, err)

	fail = true
	status, paths, err := d.Fetch(t.Context(), "test", urls)
	require.NoError(t, err, "a failed refresh with a cached copy is not an error")
	assert.False(t, status.Changed)
	require.Len(t, paths, 1)
	assert.FileExists(t, d.partPath(srv.URL), "cached list must survive a failed refresh")
}

func TestDownloader_ErrorsWhenAllSourcesFailWithoutCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	d := newTestDownloader(t)
	_, _, err := d.Fetch(t.Context(), "test", []string{srv.URL})
	assert.Error(t, err)
}

func TestDownloader_NoURLs(t *testing.T) {
	d := newTestDownloader(t)
	status, paths, err := d.Fetch(t.Context(), "test", nil)
	require.NoError(t, err)
	assert.False(t, status.Changed)
	assert.Empty(t, paths)
}

func TestDownloader_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	d := newTestDownloader(t)
	_, _, err := d.Fetch(ctx, "test", []string{srv.URL})
	assert.Error(t, err)
}

func TestLooksLikeHTML(t *testing.T) {
	assert.True(t, looksLikeHTML("<!DOCTYPE html>\n<html>"))
	assert.True(t, looksLikeHTML("  <html lang=\"en\">"))
	assert.True(t, looksLikeHTML("<!doctype HTML>"))
	assert.False(t, looksLikeHTML("# comment\nads.example.com"))
	assert.False(t, looksLikeHTML("0.0.0.0 ads.example.com"))
}

func TestDownloader_ReorderingKeepsCache(t *testing.T) {
	var downloads atomic.Int32
	handler := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("If-None-Match") == testETag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			downloads.Add(1)
			w.Header().Set("ETag", testETag)
			io.WriteString(w, body) //nolint:errcheck // test
		}
	}
	first := httptest.NewServer(handler(sampleList()))
	defer first.Close()
	second := httptest.NewServer(handler(strings.ReplaceAll(sampleList(), "ads", "trackers")))
	defer second.Close()

	d := newTestDownloader(t)
	_, paths, err := d.Fetch(t.Context(), "test", []string{first.URL, second.URL})
	require.NoError(t, err)
	require.Len(t, paths, 2)
	require.Equal(t, int32(2), downloads.Load())

	status, reordered, err := d.Fetch(t.Context(), "test", []string{second.URL, first.URL})
	require.NoError(t, err)
	assert.False(t, status.Changed)
	assert.Equal(t, int32(2), downloads.Load(), "reordering must not re-download")
	assert.Equal(t, []string{paths[1], paths[0]}, reordered, "each URL keeps its own file")
}

func TestDownloader_URLMovedBetweenListsKeepsCache(t *testing.T) {
	var downloads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == testETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		downloads.Add(1)
		w.Header().Set("ETag", testETag)
		io.WriteString(w, sampleList()) //nolint:errcheck // test
	}))
	defer srv.Close()

	d := newTestDownloader(t)
	_, before, err := d.Fetch(t.Context(), "ads", []string{srv.URL})
	require.NoError(t, err)

	_, after, err := d.Fetch(t.Context(), "some-other-list", []string{srv.URL})
	require.NoError(t, err)

	assert.Equal(t, before, after, "the cache slot follows the URL, not the list")
	assert.Equal(t, int32(1), downloads.Load(), "moving a URL must not re-download it")
}
