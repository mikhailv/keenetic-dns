package routing

import (
	"cmp"
	"context"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/agent"
	agentv1 "github.com/mikhailv/keenetic-dns/agent/rpc/v1"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/storage"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type IPRouteController struct {
	cfg            *config.Dynamic[*config.Routing]
	logger         *slog.Logger
	dnsStore       *storage.DNSStore
	networkService agent.NetworkServiceClient
	routes         util.Set[IPRoute]
	routesMu       sync.RWMutex
	reconcileMu    sync.Mutex
	reconcileCh    chan struct{}
}

func NewIPRouteController(
	cfg *config.Dynamic[*config.Routing],
	logger *slog.Logger,
	dnsStore *storage.DNSStore,
	networkService agent.NetworkServiceClient,
) *IPRouteController {
	return &IPRouteController{
		cfg:            cfg,
		logger:         logger,
		dnsStore:       dnsStore,
		networkService: networkService,
		reconcileCh:    make(chan struct{}, 1),
	}
}

func (s *IPRouteController) LookupHost(host string) (ok bool, pattern, iface string) {
	cfg := s.cfg.Get()
	if pattern = cfg.LookupHost(host); pattern != "" {
		return true, pattern, cfg.Rule.Oif
	}
	return false, "", ""
}

func (s *IPRouteController) LookupIP(ip types.IPv4) (ok bool, pattern, iface string) {
	cfg := s.cfg.Get()
	if pattern = cfg.LookupIP(ip); pattern != "" {
		return true, pattern, cfg.Rule.Oif
	}
	return false, "", ""
}

func (s *IPRouteController) Routes() []IPRouteDNS {
	s.routesMu.RLock()
	defer s.routesMu.RUnlock()
	cfg := s.cfg.Get()
	res := make([]IPRouteDNS, 0, s.routes.Size())
	for _, route := range s.routes.Values() {
		records := removeExpiredRecords(s.dnsStore.LookupIP(route.Addr), cfg.RouteTimeout)
		slices.SortFunc(records, func(a, b types.DNSRecord) int {
			return cmp.Compare(a.Domain, b.Domain)
		})
		res = append(res, IPRouteDNS{route, records})
	}
	return res
}

func (s *IPRouteController) Start(ctx context.Context) {
	s.cfg.Listen(func() { s.onConfigUpdated(ctx) })
	s.reconcile(ctx)
	go s.startReconcileLoop(ctx)
}

func (s *IPRouteController) onConfigUpdated(ctx context.Context) {
	select {
	case s.reconcileCh <- struct{}{}:
	case <-ctx.Done():
	}
}

func (s *IPRouteController) makeRoute(cfg *config.Routing, ip types.IPv4) IPRoute {
	return IPRoute{cfg.Rule.Table, cfg.Rule.Oif, ip}
}

func (s *IPRouteController) startReconcileLoop(ctx context.Context) {
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
	s.dnsStore.RemoveExpired(cfg.RouteTimeout)
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

	rule := IPRoutingRule(cfg.Rule)
	res, err := s.networkService.HasRule(ctx, &agentv1.HasRuleReq{
		Rule: mapToAgentRule(rule),
	})
	if err != nil {
		s.logger.Error("failed to check if rule exists", "err", err, "", rule)
	} else if !res.Exists {
		s.addRule(ctx, rule)
	}
}

func (s *IPRouteController) reconcileRoutes(ctx context.Context, cfg *config.Routing) {
	defer metrics.TrackDuration("reconcile_routes")()
	defer log.Profile(s.logger, "reconcile routes")()

	s.routesMu.Lock()
	defer s.routesMu.Unlock()

	definedRoutes := s.loadRoutes(ctx, cfg.Rule.Table)
	unknownRoutes := maps.Clone(definedRoutes)

	addRoute := func(route IPRoute) {
		if _, defined := definedRoutes[route]; defined {
			delete(unknownRoutes, route) // route is defined, delete it from set of unknown routes
			s.routes.Add(route)
		} else {
			s.addRoute(ctx, route)
		}
	}

	for _, route := range s.routes.Values() {
		// lookup known DNS records by IP address
		records := s.dnsStore.LookupIP(route.Addr)
		// exclude already expired DNS records
		records = removeExpiredRecords(records, cfg.RouteTimeout)
		// exclude DNS records which should not be routed
		records = slices.DeleteFunc(records, func(rec types.DNSRecord) bool {
			return cfg.LookupHost(rec.Domain) == ""
		})
		if len(records) > 0 {
			addRoute(route)
		}
	}

	for _, rec := range s.dnsStore.Records() {
		if rec.Expired(cfg.RouteTimeout) {
			continue
		}
		route := s.makeRoute(cfg, rec.IP)
		if s.routes.Has(route) {
			continue
		}
		if cfg.LookupHost(rec.Domain) != "" {
			addRoute(route)
		}
	}

	for _, addr := range cfg.Static {
		addRoute(s.makeRoute(cfg, addr))
	}

	for route := range unknownRoutes {
		s.deleteRoute(ctx, route)
	}
}

func (s *IPRouteController) AddRoute(ctx context.Context, ip types.IPv4) {
	s.routesMu.Lock()
	defer s.routesMu.Unlock()
	route := s.makeRoute(s.cfg.Get(), ip)
	if !s.routes.Has(route) {
		s.addRoute(ctx, route)
	}
}

func (s *IPRouteController) addRoute(ctx context.Context, route IPRoute) {
	defer metrics.TrackDuration("add_route")()

	_, err := s.networkService.AddRoute(ctx, &agentv1.AddRouteReq{
		Route: mapToAgentRoute(route),
	})
	if err != nil {
		s.logger.Error("failed to add route", "err", err, "", route)
	} else {
		s.logger.Info("route added", "", route)
	}
	// add in any way, it will be re-added on next reconcile iteration
	s.routes.Add(route)
}

func (s *IPRouteController) deleteRoute(ctx context.Context, route IPRoute) {
	defer metrics.TrackDuration("delete_route")()

	_, err := s.networkService.DeleteRoute(ctx, &agentv1.DeleteRouteReq{
		Route: mapToAgentRoute(route),
	})
	if err != nil {
		s.logger.Error("failed to delete route", "err", err, "", route)
	} else {
		s.logger.Info("route deleted", "", route)
		s.routes.Remove(route)
	}
}

func (s *IPRouteController) addRule(ctx context.Context, rule IPRoutingRule) {
	defer metrics.TrackDuration("add_rule")()

	_, err := s.networkService.AddRule(ctx, &agentv1.AddRuleReq{
		Rule: mapToAgentRule(rule),
	})
	if err != nil {
		s.logger.Error("failed to add rule", "err", err, "", rule)
	} else {
		s.logger.Info("rule added", "", rule)
	}
}

func (s *IPRouteController) loadRoutes(ctx context.Context, tableId int) util.Set[IPRoute] {
	defer metrics.TrackDuration("load_routes")()

	res, err := s.networkService.ListRoutes(ctx, &agentv1.ListRoutesReq{Table: uint32(tableId)})
	if err != nil {
		s.logger.Error("failed to load route table", "err", err, "table", tableId)
		return nil
	}

	routes := make(util.Set[IPRoute], len(res.Routes))
	for _, it := range res.Routes {
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

func mapToAgentRule(rule IPRoutingRule) *agentv1.Rule {
	return &agentv1.Rule{
		Table:    uint32(rule.Table),
		Iif:      rule.Iif,
		Priority: uint32(rule.Priority),
	}
}

func mapToAgentRoute(route IPRoute) *agentv1.Route {
	return &agentv1.Route{
		Table:   uint32(route.Table),
		Iface:   route.Iface,
		Address: route.Addr.String(),
	}
}

func removeExpiredRecords(records []types.DNSRecord, extraTTL time.Duration) []types.DNSRecord {
	return slices.DeleteFunc(records, func(rec types.DNSRecord) bool {
		return rec.Expired(extraTTL)
	})
}
