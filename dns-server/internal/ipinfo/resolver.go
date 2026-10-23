package ipinfo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/lookup"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

const maxBatchSize = 1000

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

type Info struct {
	Network string `json:"network"`
	ASN     uint32 `json:"asn,omitempty"`
	Org     string `json:"org,omitempty"`
	Country string `json:"country,omitempty"`
	City    string `json:"city,omitempty"`
}

type entry struct {
	info    Info
	fetched time.Time
}

type cacheState struct {
	ranges  lookup.IPRangeMap[*entry]
	entries map[types.IPv4]*entry
}

type Resolver struct {
	cfg              Config
	logger           *slog.Logger
	client           *http.Client
	now              func() time.Time
	state            atomic.Pointer[cacheState]
	mu               sync.Mutex
	unavailableUntil time.Time
}

func NewResolver(cfg Config, logger *slog.Logger) *Resolver {
	r := &Resolver{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{Timeout: cfg.Timeout},
		now:    time.Now,
	}
	r.state.Store(&cacheState{entries: map[types.IPv4]*entry{}})
	return r
}

func (r *Resolver) Enabled() bool {
	return r != nil && r.cfg.URL != ""
}

func (r *Resolver) Lookup(ctx context.Context, ips []types.IPv4) map[types.IPv4]Info {
	res := make(map[types.IPv4]Info, len(ips))
	if !r.Enabled() {
		return res
	}
	missing := r.fromCache(ips, res)
	if len(missing) == 0 {
		return res
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.now().Before(r.unavailableUntil) {
		return res
	}
	missing = r.fromCache(missing, res)
	if len(missing) == 0 {
		return res
	}

	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()
	fetched := make(map[types.IPv4]*entry)
	for start := 0; start < len(missing); start += maxBatchSize {
		batch := missing[start:min(start+maxBatchSize, len(missing))]
		if err := r.fetch(ctx, batch, fetched); err != nil {
			r.unavailableUntil = r.now().Add(r.cfg.RetryAfter)
			r.logger.Warn("ipinfo lookup failed, retrying later", "err", err, "retry_after", r.cfg.RetryAfter)
			break
		}
	}
	if len(fetched) > 0 {
		r.store(fetched)
		r.fromCache(missing, res)
	}
	return res
}

func (r *Resolver) fromCache(ips []types.IPv4, res map[types.IPv4]Info) []types.IPv4 {
	state := r.state.Load()
	now := r.now()
	var missing []types.IPv4
	seen := make(map[types.IPv4]struct{}, len(ips))
	for _, ip := range ips {
		if _, ok := seen[ip]; ok || !isPublic(ip) {
			continue
		}
		seen[ip] = struct{}{}
		if e, ok := state.ranges.Get(ip); ok && now.Sub(e.fetched) < r.cfg.TTL {
			res[ip] = e.info
		} else {
			missing = append(missing, ip)
		}
	}
	return missing
}

func (r *Resolver) store(fetched map[types.IPv4]*entry) {
	old := r.state.Load()
	now := r.now()
	entries := make(map[types.IPv4]*entry, len(old.entries)+len(fetched))
	for network, e := range old.entries {
		if now.Sub(e.fetched) < r.cfg.TTL {
			entries[network] = e
		}
	}
	maps.Copy(entries, fetched)
	b := lookup.NewIPRangeMapBuilder[*entry]()
	for network, e := range entries {
		b.Add(network, e)
	}
	r.state.Store(&cacheState{ranges: b.Build(), entries: entries})
}

type batchItem struct {
	IP  string `json:"ip"`
	Geo *struct {
		Network string `json:"network"`
		ASN     *struct {
			Number       uint32 `json:"number"`
			Organization string `json:"organization"`
		} `json:"asn"`
		Country *struct {
			ISOCode string `json:"iso_code"`
		} `json:"country"`
		City *struct {
			Name string `json:"name"`
		} `json:"city"`
	} `json:"geo"`
}

func (r *Resolver) fetch(ctx context.Context, ips []types.IPv4, out map[types.IPv4]*entry) (resErr error) {
	body := make([]string, len(ips))
	for i, ip := range ips {
		body[i] = ip.String()
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(r.cfg.URL, "/")+"/ip/batch", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		resErr = errors.Join(resErr, resp.Body.Close())
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	var items []batchItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fetched := r.now()
	for _, it := range items {
		network, info, err := it.toInfo()
		if err != nil {
			return err
		}
		out[network] = &entry{info: info, fetched: fetched}
	}
	return nil
}

func (it batchItem) toInfo() (types.IPv4, Info, error) {
	if it.Geo == nil || it.Geo.Network == "" {
		ip, err := types.ParseIPv4(it.IP)
		return ip, Info{Network: it.IP + "/32"}, err
	}
	network, err := types.ParseIPv4(it.Geo.Network)
	if err != nil {
		return types.IPv4{}, Info{}, err
	}
	info := Info{Network: it.Geo.Network}
	if it.Geo.ASN != nil {
		info.ASN = it.Geo.ASN.Number
		info.Org = it.Geo.ASN.Organization
	}
	if it.Geo.Country != nil {
		info.Country = it.Geo.Country.ISOCode
	}
	if it.Geo.City != nil {
		info.City = it.Geo.City.Name
	}
	return network, info, nil
}

func isPublic(ip types.IPv4) bool {
	addr := netip.AddrFrom4([4]byte(ip[:4]))
	return addr.IsGlobalUnicast() && !addr.IsPrivate() && !cgnat.Contains(addr)
}
