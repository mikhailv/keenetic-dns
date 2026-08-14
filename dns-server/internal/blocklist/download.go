package blocklist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	maxListSize = 256 << 20

	//
	minListValidRulesFraction = 0.25
)

type downloadMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	FetchedAt    string `json:"fetched_at"`
	Rules        int    `json:"rules"`
}

type Downloader struct {
	dir    string
	client *http.Client
	logger *slog.Logger
}

func NewDownloader(dir string, timeout time.Duration, logger *slog.Logger) *Downloader {
	return &Downloader{
		dir:    dir,
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}
}

const urlKeyLen = 6

func urlKey(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:urlKeyLen])
}

func (d *Downloader) partPath(url string) string {
	return filepath.Join(d.dir, urlKey(url)+".txt")
}

func (d *Downloader) partMetaPath(url string) string {
	return filepath.Join(d.dir, urlKey(url)+".meta.json")
}

func (d *Downloader) CachedPaths(urls []string) []string {
	paths := make([]string, 0, len(urls))
	for _, url := range urls {
		path := d.partPath(url)
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}

type FetchStatus struct {
	Changed  bool
	Degraded bool
}

func (d *Downloader) Fetch(ctx context.Context, label string, urls []string) (status FetchStatus, paths []string, err error) {
	if len(urls) == 0 {
		return status, nil, nil
	}
	var errs []error
	for _, url := range urls {
		if err := ctx.Err(); err != nil {
			return status, paths, err
		}
		path := d.partPath(url)
		metaPath := d.partMetaPath(url)

		prev, _ := d.loadMeta(metaPath)
		if prev != nil && prev.URL != url { // compare urls to be sure that there is not hash collision
			prev = nil
		}
		if _, statErr := os.Stat(path); statErr != nil {
			prev = nil
		}

		updated, err := d.fetchOne(ctx, path, metaPath, url, prev)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", url, err))
			if _, statErr := os.Stat(path); statErr == nil {
				d.logger.Error("blocklist source failed, using previously downloaded copy",
					"list", label, "url", url, "err", err)
				status.Degraded = true
				paths = append(paths, path)
			} else {
				d.logger.Error("blocklist source failed and was never downloaded, skipping it",
					"list", label, "url", url, "err", err)
			}
		case updated:
			d.logger.Info("blocklist source updated", "list", label, "url", url)
			status.Changed = true
			paths = append(paths, path)
		default:
			d.logger.Debug("blocklist source unchanged", "list", label, "url", url)
			paths = append(paths, path)
		}
	}

	if len(paths) == 0 {
		return FetchStatus{}, nil, fmt.Errorf("list %q: %w", label, errors.Join(errs...))
	}
	return status, paths, nil
}

func (d *Downloader) fetchOne(
	ctx context.Context,
	path, metaPath, url string,
	prev *downloadMeta,
) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	if prev != nil {
		if prev.ETag != "" {
			req.Header.Set("If-None-Match", prev.ETag)
		}
		if prev.LastModified != "" {
			req.Header.Set("If-Modified-Since", prev.LastModified)
		}
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("unexpected status %s", resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); isHTML(ct) {
		return false, fmt.Errorf("unexpected content type %q", ct)
	}

	rules, err := d.saveBody(path, resp.Body)
	if err != nil {
		return false, err
	}

	meta := downloadMeta{
		URL:          url,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		FetchedAt:    time.Now().UTC().Format(time.RFC3339),
		Rules:        rules,
	}
	if err := d.saveMeta(metaPath, meta); err != nil {
		d.logger.Warn("failed to save blocklist metadata", "path", metaPath, "err", err)
	}
	return true, nil
}

func (d *Downloader) saveBody(path string, body io.Reader) (int, error) {
	var rules int
	err := util.SaveToFile(path, util.SaveFileConfig{
		Reader: body,
		Limit:  maxListSize,
		BeforeCommit: func(tmpFile string) error {
			var err error
			rules, err = validateList(tmpFile)
			return err
		},
	})
	return rules, err
}

func validateList(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	head := make([]byte, 512)
	n, err := f.Read(head)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if looksLikeHTML(string(head[:n])) {
		return 0, errors.New("response body is an HTML document")
	}
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return 0, seekErr
	}

	rules, attempts, err := Scan(f)
	if err != nil {
		return 0, err
	}
	if attempts == 0 {
		return 0, errors.New("response body holds no rules at all")
	}
	if fraction := float64(rules) / float64(attempts); fraction < minListValidRulesFraction {
		return 0, fmt.Errorf("only %d rules parsed from %d candidates (%.1f%%)", rules, attempts, fraction*100)
	}
	return rules, nil
}

func isHTML(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/html")
}

func looksLikeHTML(head string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(head))
	return strings.HasPrefix(trimmed, "<!doctype") || strings.HasPrefix(trimmed, "<html")
}

func (d *Downloader) loadMeta(path string) (*downloadMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m downloadMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *Downloader) saveMeta(path string, meta downloadMeta) error {
	return util.SaveToFileFunc(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(meta)
	})
}
