package routing

import (
	"cmp"
	"context"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/agentclient"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/storage"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type IPRouteController struct {
	cfg            *util.Dynamic[*config.Routing]
	logger         *slog.Logger
	dnsStore       *storage.DNSStore
	networkService agentclient.NetworkServiceClient
	lookups        *storage.LookupIndex
	routes         util.SyncMap[IPRoute, IPRouteInfo]
	reconcileMu    sync.Mutex
	reconcileCh    chan struct{}
}

func NewIPRouteController(
	cfg *util.Dynamic[*config.Routing],
	logger *slog.Logger,
	dnsStore *storage.DNSStore,
	networkService agentclient.NetworkServiceClient,
	routeTimeout time.Duration,
) *IPRouteController {
	return &IPRouteController{
		cfg:            cfg,
		logger:         logger,
		dnsStore:       dnsStore,
		networkService: networkService,
		lookups:        storage.NewLookupIndex(routeTimeout),
		reconcileCh:    make(chan struct{}, 1),
	}
}

func (s *IPRouteController) lookupHost(host string) (ok bool, pattern, iface string) {
	cfg := s.cfg.Get()
	if pattern = cfg.LookupHost(host); pattern != "" {
		return true, pattern, cfg.Oif
	}
	return false, "", ""
}

func (s *IPRouteController) lookupIP(ip types.IPv4) (ok bool, pattern, iface string) {
	cfg := s.cfg.Get()
	if pattern = cfg.LookupIP(ip); pattern != "" {
		return true, pattern, cfg.Oif
	}
	return false, "", ""
}

func (s *IPRouteController) lookupIgnoredHost(host string) (ok bool, pattern string) {
	cfg := s.cfg.Get()
	if pattern = cfg.LookupIgnoredHost(host); pattern != "" {
		return true, pattern
	}
	return false, ""
}

func (s *IPRouteController) Routes(withLookups bool) []IPRouteDNS {
	res := make([]IPRouteDNS, 0, s.routes.Size())
	for route, info := range s.routes.Snapshot() {
		lookups := s.lookups.LookupByIP(route.Addr)
		recordSet := make(map[string]DNSRecord, len(lookups))
		for _, l := range lookups {
			if _, ok := recordSet[l.Domain]; ok {
				continue
			}
			for _, it := range l.IPs {
				if it.IP != route.Addr {
					continue
				}
				recordSet[l.Domain] = newDNSRecord(l.Domain, it.IP, l.Time, int(it.TTL))
				break
			}
		}
		records := util.SeqToSlice(len(recordSet), maps.Values(recordSet))
		slices.SortFunc(records, func(a, b DNSRecord) int {
			return cmp.Compare(a.Domain, b.Domain)
		})
		if withLookups {
			res = append(res, IPRouteDNS{route, info, records, lookups})
		} else {
			res = append(res, IPRouteDNS{route, info, records, nil})
		}
	}
	return res
}

func (s *IPRouteController) Start(ctx context.Context) {
	s.cfg.Listen(func() { s.onConfigUpdated(ctx) })
	s.syncLookups()
	go s.startReconcileLoop(ctx)
}

func (s *IPRouteController) syncLookups() {
	for it := range s.dnsStore.Iterator() {
		s.lookups.Add(it)
	}
}

func (s *IPRouteController) onConfigUpdated(ctx context.Context) {
	s.syncLookups()
	select {
	case s.reconcileCh <- struct{}{}:
	case <-ctx.Done():
	}
}

func (s *IPRouteController) makeRoute(cfg *config.Routing, ip types.IPv4) IPRoute {
	return IPRoute{cfg.Table, cfg.Oif, ip}
}

func (s *IPRouteController) startReconcileLoop(ctx context.Context) {
	s.reconcile(ctx)
	for {
		cfg := s.cfg.Get()
		select {
		case <-ctx.Done():
			return
		case <-s.reconcileCh:
			s.reconcile(ctx)
		case <-time.After(cfg.Reconcile.Interval):
			s.reconcile(ctx)
		}
	}
}

func (s *IPRouteController) reconcile(ctx context.Context) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	cfg := s.cfg.Get()
	s.doReconcile(ctx, cfg, s.reconcileRules)
	s.doReconcile(ctx, cfg, s.reconcileRoutes)
}

func (s *IPRouteController) doReconcile(ctx context.Context, cfg *config.Routing, fn func(context.Context, *config.Routing)) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Reconcile.Timeout)
	defer cancel()
	fn(ctx, cfg)
}

func (s *IPRouteController) reconcileRules(ctx context.Context, cfg *config.Routing) {
	defer metrics.TrackDuration("reconcile_rules")()
	defer log.Profile(s.logger, "reconcile rules")()

	for _, r := range cfg.Rules {
		rule := IPRoutingRule{Table: cfg.Table, From: r.From, Priority: r.Priority}
		exists, err := s.networkService.HasRule(ctx, uint32(rule.Table), rule.From)
		if err != nil {
			s.logger.Error("failed to check if rule exists", "err", err, "", rule)
			continue
		}
		if !exists {
			s.addRule(ctx, rule)
		}
	}
}

func (s *IPRouteController) reconcileRoutes(ctx context.Context, cfg *config.Routing) {
	defer metrics.TrackDuration("reconcile_routes")()
	defer log.Profile(s.logger, "reconcile routes")()

	for _, it := range s.lookups.RemoveExpired() {
		s.logger.Info("removed expired lookup", "domain", it.Domain,
			"added", formatAgo(it.Time.Time()), "resolver", it.Resolver.Name)
	}

	definedRoutes := s.loadRoutes(ctx, cfg.Table)
	actual, obsolete := s.partitionRoutes(cfg, definedRoutes)

	added := 0
	for route, info := range actual {
		if definedRoutes.Has(route) {
			s.routes.PutIfAbsent(route, info)
		} else if s.addRoute(ctx, route, info) {
			s.routes.PutIfAbsent(route, info)
			added++
		}
	}

	deleted := 0
	for route, reason := range obsolete {
		if !definedRoutes.Has(route) {
			s.routes.Remove(route)
		} else if s.deleteRoute(ctx, route, reason) {
			s.routes.Remove(route)
			deleted++
		}
	}

	s.logger.Info("routes updated", "added", added, "deleted", deleted,
		"routes", s.routes.Size(), "lookups", s.lookups.Size())
}

// partitionRoutes splits routes into two sets: actual (should exist) and obsolete (should be removed).
func (s *IPRouteController) partitionRoutes(
	cfg *config.Routing,
	definedRoutes util.Set[IPRoute],
) (actual map[IPRoute]IPRouteInfo, obsolete map[IPRoute]string) {
	actual = make(map[IPRoute]IPRouteInfo, s.routes.Size())
	obsolete = make(map[IPRoute]string, 10)

	tsNow := types.TimestampFromTime(time.Now())

	// Static routes always belong to actual.
	for _, addr := range cfg.Static {
		actual[s.makeRoute(cfg, addr)] = IPRouteInfo{"", "static", tsNow}
	}

	// Partition routes from DNS records.
	for _, dl := range s.lookups.Values() {
		res := s.resolveRouting(dl)
		if !res.Has(types.ActionRouted) {
			s.lookups.Remove(dl)
			continue
		}
		for _, it := range dl.IPs {
			route := s.makeRoute(cfg, it.IP)
			if _, ok := actual[route]; ok {
				continue
			}
			r, found := res[it.IP]
			if found && r.Action == types.ActionRouted && !r.Static {
				actual[route] = IPRouteInfo{dl.Domain, r.Reason, dl.Time}
			}
		}
	}

	// Routes defined on router but not in actual are expired.
	for route := range definedRoutes {
		if _, ok := actual[route]; !ok {
			obsolete[route] = "expired"
		}
	}

	// Routes in local cache but not in either set are obsolete.
	for route := range s.routes.Iterator() {
		if _, ok := actual[route]; !ok && obsolete[route] == "" {
			obsolete[route] = "obsolete"
		}
	}

	return actual, obsolete
}

func (s *IPRouteController) AddRoutes(ctx context.Context, lookup *types.DomainLookup) types.IPRoutings {
	res := s.resolveRouting(lookup)
	if !res.Has(types.ActionRouted) {
		return res
	}
	s.lookups.Add(lookup)
	cfg := s.cfg.Get()
	for ip, it := range res {
		if it.Static || it.Action != types.ActionRouted {
			continue
		}
		route := s.makeRoute(cfg, ip)
		if s.routes.Has(route) {
			continue
		}
		info := IPRouteInfo{lookup.Domain, it.Reason, lookup.Time}
		if s.addRoute(ctx, route, info) {
			s.routes.PutIfAbsent(route, info)
			it.Added = true
			res[ip] = it
		}
	}
	return res
}

func (s *IPRouteController) addRoute(ctx context.Context, route IPRoute, info IPRouteInfo) bool {
	defer metrics.TrackDuration("add_route")()

	err := s.networkService.AddRoute(ctx, mapToAgentRoute(route))
	if err != nil {
		s.logger.Error("failed to add route", "err", err, "", route)
		return false
	}
	s.logger.Info("route added", "", route, "domain", info.Domain, "reason", info.Reason)
	return true
}

func (s *IPRouteController) deleteRoute(ctx context.Context, route IPRoute, reason string) bool {
	defer metrics.TrackDuration("delete_route")()

	err := s.networkService.DeleteRoute(ctx, mapToAgentRoute(route))
	if err != nil {
		s.logger.Error("failed to delete route", "err", err, "", route)
		return false
	}
	if info, ok := s.routes.Get(route); ok {
		s.logger.Info("route deleted", "", route, "reason", reason, "domain", info.Domain, "added", formatAgo(info.AddedAt.Time()))
	} else {
		s.logger.Info("route deleted", "", route, "reason", reason)
	}
	return true
}

func (s *IPRouteController) addRule(ctx context.Context, rule IPRoutingRule) bool {
	defer metrics.TrackDuration("add_rule")()

	err := s.networkService.AddRule(ctx, mapToAgentRule(rule))
	if err != nil {
		s.logger.Error("failed to add rule", "err", err, "", rule)
		return false
	}
	s.logger.Info("rule added", "", rule)
	return true
}

func (s *IPRouteController) loadRoutes(ctx context.Context, tableId int) util.Set[IPRoute] {
	defer metrics.TrackDuration("load_routes")()

	res, err := s.networkService.ListRoutes(ctx, uint32(tableId))
	if err != nil {
		s.logger.Error("failed to load route table", "err", err, "table", tableId)
		return nil
	}

	routes := make(util.Set[IPRoute], len(res))
	for _, it := range res {
		addr, err := types.ParseIPv4(it.Address)
		if err != nil {
			s.logger.Warn("unexpected route address", "addr", it.Address)
			continue
		}
		route := IPRoute{int(it.Table), it.Iface, addr}
		routes.Add(route)
	}
	return routes
}

func (s *IPRouteController) resolveRouting(dl *types.DomainLookup) types.IPRoutings { //nolint:gocognit // it's ok
	var res types.IPRoutings

	allRouted := func(iface, reason string, ips []types.DomainIP) types.IPRoutings {
		for _, it := range ips {
			res.AddRoute(iface, reason, it.IP)
		}
		return res
	}

	allIgnored := func(reason string, ips []types.DomainIP) types.IPRoutings {
		for _, it := range ips {
			res.AddExcluded(reason, it.IP)
		}
		return res
	}

	if ok, pattern := s.lookupIgnoredHost(dl.Domain); ok {
		return allIgnored(pattern, dl.IPs)
	}
	if ok, pattern, iface := s.lookupHost(dl.Domain); ok {
		return allRouted(iface, pattern, dl.IPs)
	}

	for _, it := range dl.CNames {
		if ok, pattern := s.lookupIgnoredHost(it.Name); ok {
			return allIgnored("CNAME "+pattern, dl.IPs)
		}
		if ok, pattern, iface := s.lookupHost(it.Name); ok {
			return allRouted(iface, "CNAME "+pattern, dl.IPs)
		}
	}

loop:
	for _, it := range dl.IPs {
		ip := it.IP
		if ok, pattern, iface := s.lookupIP(ip); ok { // static IP
			res.AddStaticRoute(iface, pattern, ip)
			continue loop
		}
		for _, ptr := range it.PTR {
			if ok, pattern := s.lookupIgnoredHost(ptr.Name); ok {
				res.AddExcluded("PTR "+pattern, ip)
				continue loop
			}
			if ok, pattern, iface := s.lookupHost(ptr.Name); ok {
				res.AddRoute(iface, "PTR "+pattern, ip)
				continue loop
			}
		}
		for _, soa := range it.SOA {
			if ok, pattern := s.lookupIgnoredHost(soa.Name); ok {
				res.AddExcluded("SOA "+pattern, ip)
				continue loop
			}
			if ok, pattern, iface := s.lookupHost(soa.Name); ok {
				res.AddRoute(iface, "SOA "+pattern, ip)
				continue loop
			}
		}
	}

	return res
}

func mapToAgentRule(rule IPRoutingRule) agentclient.Rule {
	return agentclient.Rule{
		Table:    uint32(rule.Table),
		From:     rule.From,
		Priority: uint32(rule.Priority),
	}
}

func mapToAgentRoute(route IPRoute) agentclient.Route {
	return agentclient.Route{
		Table:   uint32(route.Table),
		Iface:   route.Iface,
		Address: route.Addr.String(),
	}
}

func formatAgo(t time.Time) string {
	d := time.Since(t)
	seconds := d.Seconds()
	if seconds >= 60 {
		return strconv.Itoa(int(seconds/60)) + " min ago"
	}
	return strconv.Itoa(int(seconds)) + " sec ago"
}
