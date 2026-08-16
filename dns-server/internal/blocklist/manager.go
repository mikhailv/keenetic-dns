package blocklist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/lookup"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const IndexFileName = "blocklist.fst"

type Manager struct {
	cfg           Config
	lists         []List
	state         []ListState
	downloader    *Downloader
	logger        *slog.Logger
	retryDelay    time.Duration
	maxRetryDelay time.Duration

	mu          sync.RWMutex
	index       *Index
	degraded    bool
	clientMasks lookup.IPTree[uint32]
	defaultMask uint32
}

func NewManager(cfg Config, logger *slog.Logger) *Manager {
	cfg.SetDefaults()
	return &Manager{
		cfg:           cfg,
		lists:         cfg.EnabledLists(),
		state:         cfg.State(),
		downloader:    NewDownloader(filepath.Join(cfg.DataDir, "lists"), cfg.DownloadTimeout, logger),
		logger:        logger,
		retryDelay:    startupRetryDelay,
		maxRetryDelay: maxStartupRetryDelay,
	}
}

func (m *Manager) IndexPath() string {
	return filepath.Join(m.cfg.DataDir, IndexFileName)
}

func (m *Manager) Lookup(domain string, clientIP types.IPv4) (Match, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.index == nil {
		return Match{}, false
	}
	return m.index.Lookup(domain, m.maskFor(clientIP))
}

func (m *Manager) maskFor(clientIP types.IPv4) uint32 {
	if mask, ok := m.clientMasks.Get(clientIP); ok {
		return mask
	}
	return m.defaultMask
}

func (m *Manager) Loaded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.index != nil
}

func (m *Manager) Degraded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.degraded
}

func (m *Manager) setDegraded(degraded bool) {
	m.mu.Lock()
	m.degraded = degraded
	m.mu.Unlock()
}

func (m *Manager) stateMatchesIndex() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.index == nil {
		return true
	}
	return slices.EqualFunc(m.index.State(), m.state, listStateEqual)
}

func listStateEqual(a, b ListState) bool {
	return a.Name == b.Name &&
		slices.Equal(a.URLs, b.URLs) &&
		slices.Equal(a.AllowURLs, b.AllowURLs) &&
		slices.Equal(a.Allow, b.Allow) &&
		slices.Equal(a.Deny, b.Deny)
}

const (
	startupRetryDelay    = 15 * time.Second
	maxStartupRetryDelay = 10 * time.Minute
)

func (m *Manager) Start(ctx context.Context) util.Waiter {
	if err := m.loadCached(); err != nil {
		m.logger.Warn("no usable blocklist index at startup, blocking inactive until refresh", "err", err)
	}

	startup, startupDone := util.NewWaiter()
	go func() {
		defer startupDone()
		m.startupRefresh(ctx)
	}()

	periodic := util.RunPeriodically(ctx.Done(), m.cfg.RefreshInterval, func() {
		if err := m.Refresh(ctx); err != nil && ctx.Err() == nil {
			m.logger.Error("blocklist refresh failed", "err", err)
		}
	})
	return util.WaitAll(startup, periodic)
}

func (m *Manager) startupRefresh(ctx context.Context) {
	for delay := m.retryDelay; ; delay *= 2 {
		if err := m.Refresh(ctx); err != nil && ctx.Err() == nil {
			m.logger.Error("blocklist refresh failed", "err", err)
		}
		if ctx.Err() != nil {
			return
		}

		switch {
		case !m.Loaded():
			m.logger.Warn("blocking inactive, retrying", "retry_in", delay)
		case m.Degraded():
			m.logger.Warn("blocklist served from an older copy, retrying", "retry_in", delay)
		default:
			return
		}
		if delay > m.maxRetryDelay {
			m.logger.Warn("giving up on the startup retries, leaving it to the refresh interval",
				"refresh_interval", m.cfg.RefreshInterval)
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (m *Manager) loadCached() error {
	if !m.indexIsFresh() {
		return errors.New("cached index is missing or older than its source lists")
	}
	index, err := Open(m.IndexPath())
	if err != nil {
		return err
	}
	if !slices.EqualFunc(index.State(), m.state, listStateEqual) {
		_ = index.Close()
		return errors.New("cached index was built from a different configuration")
	}
	masks, defaultMask := m.buildClientMasks(index.ListIDs())
	m.swap(index, masks, defaultMask)
	m.logger.Info("blocklist index loaded from cache",
		"domains", index.Domains(), "built_at", index.BuiltAt())
	return nil
}

func (m *Manager) indexIsFresh() bool {
	indexInfo, err := os.Stat(m.IndexPath())
	if err != nil {
		return false
	}
	if _, err := os.Stat(ManifestPath(m.IndexPath())); err != nil {
		return false
	}
	for _, list := range m.lists {
		for _, set := range urlSets(list) {
			paths := m.downloader.CachedPaths(set.urls)
			if len(paths) != len(set.urls) {
				return false
			}
			for _, path := range paths {
				info, err := os.Stat(path)
				if err != nil || info.ModTime().After(indexInfo.ModTime()) {
					return false
				}
			}
		}
	}
	return true
}

func (m *Manager) Refresh(ctx context.Context) error {
	if len(m.lists) == 0 {
		return nil
	}

	var changed, degraded bool
	var errs []error
	var failedLists int
	for _, src := range m.lists {
		if err := ctx.Err(); err != nil {
			return err
		}
		var listFailed bool
		for _, set := range urlSets(src) {
			status, _, err := m.downloader.Fetch(ctx, set.label, set.urls)
			if err != nil {
				errs = append(errs, err)
				listFailed = true
				continue
			}
			changed = changed || status.Changed
			degraded = degraded || status.Degraded
		}
		if listFailed {
			failedLists++
		}
	}
	m.setDegraded(degraded || len(errs) > 0)

	if failedLists == len(m.lists) {
		return fmt.Errorf("all blocklists failed: %w", errors.Join(errs...))
	}
	changed = changed || !m.stateMatchesIndex()
	if !changed && m.Loaded() {
		m.logger.Debug("blocklists unchanged, keeping index")
		return errors.Join(errs...)
	}

	if err := m.rebuild(ctx); err != nil {
		return errors.Join(append(errs, err)...)
	}
	return errors.Join(errs...)
}

func (m *Manager) rebuild(ctx context.Context) error {
	start := time.Now()
	builder := NewBuilder()
	builder.SetState(m.state)

	listIDs := map[string]int{}
	var loaded int
	for _, src := range m.lists {
		if err := ctx.Err(); err != nil {
			return err
		}
		listID, rules, err := m.addList(builder, src)
		if err != nil {
			m.logger.Error("failed to read cached blocklist", "list", src.Name, "err", err)
			continue
		}
		listIDs[src.Name] = listID
		loaded++
		m.logger.Debug("blocklist loaded", "list", src.Name, "rules", rules)
	}
	if loaded == 0 {
		return errors.New("no blocklists could be read")
	}
	if err := builder.Write(m.IndexPath(), time.Now()); err != nil {
		return err
	}
	index, err := Open(m.IndexPath())
	if err != nil {
		return fmt.Errorf("open rebuilt index: %w", err)
	}
	masks, defaultMask := m.buildClientMasks(listIDs)
	domains, regexps := index.Domains(), builder.RegexpCount()
	m.swap(index, masks, defaultMask)

	m.logger.Info("blocklist index rebuilt",
		"lists", loaded, "domains", domains, "regexps", regexps,
		"groups", len(m.cfg.Groups), "took", time.Since(start).Round(time.Millisecond))
	return nil
}

type urlSet struct {
	label  string
	urls   []string
	action Action
}

func urlSets(list List) []urlSet {
	return []urlSet{
		{label: list.Name, urls: list.URLs, action: Block},
		{label: list.Name + " (allow)", urls: list.AllowURLs, action: Allow},
	}
}

func (m *Manager) addList(builder *Builder, src List) (listID, rules int, err error) {
	listID, err = builder.AddList(src.Name, firstURL(src))
	if err != nil {
		return 0, 0, err
	}

	for _, set := range urlSets(src) {
		for _, path := range m.downloader.CachedPaths(set.urls) {
			n, err := addListFile(builder, path, listID, set.action)
			rules += n
			if err != nil {
				m.logger.Error("failed to read cached blocklist part",
					"list", src.Name, "path", path, "err", err)
			}
		}
	}
	rules += addDomains(builder, src.Deny, Block, listID)
	rules += addDomains(builder, src.Allow, Allow, listID)

	if rules == 0 {
		return listID, 0, fmt.Errorf("list %q yielded no rules", src.Name)
	}
	return listID, rules, nil
}

func addListFile(builder *Builder, path string, listID int, action Action) (rules int, resErr error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	for rule, err := range Parse(f) {
		if err != nil {
			return rules, err
		}
		if action == Allow {
			rule.Action = Allow
		}
		builder.Add(rule, listID)
		rules++
	}
	return rules, nil
}

func addDomains(builder *Builder, domains []string, action Action, listID int) (rules int) {
	for _, domain := range domains {
		if builder.AddDomain(domain, action, listID) {
			rules++
		}
	}
	return rules
}

func (m *Manager) buildClientMasks(listIDs map[string]int) (lookup.IPTree[uint32], uint32) {
	var all uint32
	for _, id := range listIDs {
		all |= uint32(1) << id
	}

	tb := lookup.NewIPTreeBuilder[uint32]()
	for _, group := range m.cfg.Groups {
		var mask uint32
		for _, name := range group.Lists {
			id, ok := listIDs[name]
			if !ok {
				m.logger.Warn("blocking group names an unknown list",
					"group", group.Name, "list", name)
				continue
			}
			mask |= uint32(1) << id
		}
		for _, client := range group.Clients {
			ip, err := types.ParseIPv4(client)
			if err != nil {
				m.logger.Error("invalid client address in blocking group",
					"group", group.Name, "client", client, "err", err)
				continue
			}
			tb.Add(ip, mask)
		}
	}
	return tb.Build(), all
}

func (m *Manager) swap(index *Index, masks lookup.IPTree[uint32], defaultMask uint32) {
	m.mu.Lock()
	previous := m.index
	m.index = index
	m.clientMasks = masks
	m.defaultMask = defaultMask
	m.mu.Unlock()

	if previous != nil {
		if err := previous.Close(); err != nil {
			m.logger.Warn("failed to release previous blocklist index", "err", err)
		}
	}
}

func (m *Manager) Close() error {
	m.mu.Lock()
	index := m.index
	m.index = nil
	m.mu.Unlock()

	if index == nil {
		return nil
	}
	return index.Close()
}

func firstURL(src List) string {
	if len(src.URLs) == 0 {
		return ""
	}
	return src.URLs[0]
}
