package internal

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"

	"github.com/mikhailv/keenetic-dns/agent"
	agentv1 "github.com/mikhailv/keenetic-dns/agent/rpc/v1"
	"github.com/mikhailv/keenetic-dns/internal/log"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type IPRouteController struct {
	cfg               atomic.Pointer[RoutingConfig]
	tableId           int
	rule              IPRoutingRule
	logger            *slog.Logger
	dnsStore          *DNSStore
	networkService    agent.NetworkServiceClient
	routes            util.Set[IPRoute]
	routesMu          sync.RWMutex
	reconcileMu       sync.Mutex
	reconcileInterval time.Duration
	reconcileTimeout  time.Duration
}

func NewIPRouteController(
	cfg RoutingConfig,
	logger *slog.Logger,
	dnsStore *DNSStore,
	networkService agent.NetworkServiceClient,
	reconcileInterval time.Duration,
	reconcileTimeout time.Duration,
) *IPRouteController {
	s := &IPRouteController{
		tableId:           cfg.Rule.Table,
		rule:              IPRoutingRule(cfg.Rule),
		logger:            logger,
		dnsStore:          dnsStore,
		networkService:    networkService,
		reconcileInterval: reconcileInterval,
		reconcileTimeout:  reconcileTimeout,
	}
	s.cfg.Store(&cfg)
	return s
}

func (s *IPRouteController) LookupHost(host string) (iface string) {
	return s.cfg.Load().LookupHost(host)
}

func (s *IPRouteController) Routes() []IPRouteDNS {
	s.routesMu.RLock()
	defer s.routesMu.RUnlock()
	cfg := s.cfg.Load()
	res := make([]IPRouteDNS, 0, s.routes.Size())
	for _, route := range s.routes.Values() {
		records := removeExpiredRecords(s.dnsStore.LookupIP(route.Addr), cfg.RouteTimeout)
		slices.SortFunc(records, func(a, b DNSRecord) int {
			return cmp.Compare(a.Domain, b.Domain)
		})
		res = append(res, IPRouteDNS{route, records})
	}
	return res
}

func (s *IPRouteController) Start(ctx context.Context) {
	s.init(s.cfg.Load())
	s.reconcile(ctx)
	go util.RunPeriodically(ctx, s.reconcileInterval, s.reconcile)
}

func (s *IPRouteController) UpdateConfig(ctx context.Context, cfg RoutingConfig) {
	old := s.cfg.Load()
	cfg.Rule = old.Rule // rule can't be updated
	s.cfg.Store(&cfg)
	s.logger.Info("routing config updated")
	s.reconcile(ctx)
}

func (s *IPRouteController) init(cfg *RoutingConfig) {
	for _, rec := range s.dnsStore.Records() {
		if iface := cfg.LookupHost(rec.Domain); iface != "" {
			s.routes.Add(IPRoute{cfg.Rule.Table, iface, rec.IP})
		}
	}
}

func (s *IPRouteController) reconcile(ctx context.Context) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	cfg := s.cfg.Load()
	s.dnsStore.RemoveExpired(cfg.RouteTimeout)
	s.doReconcile(ctx, cfg, s.reconcileRules)
	s.doReconcile(ctx, cfg, s.reconcileRoutes)
}

func (s *IPRouteController) doReconcile(ctx context.Context, cfg *RoutingConfig, fn func(context.Context, *RoutingConfig)) {
	ctx, cancel := context.WithTimeout(ctx, s.reconcileTimeout)
	defer cancel()
	fn(ctx, cfg)
}

func (s *IPRouteController) reconcileRules(ctx context.Context, cfg *RoutingConfig) {
	defer TrackDuration("reconcile_rules")()
	defer log.Profile(s.logger, "reconcile rules")()

	rule := IPRoutingRule(cfg.Rule)
	res, err := s.networkService.HasRule(ctx, connect.NewRequest(&agentv1.HasRuleReq{
		Rule: mapToAgentRule(rule),
	}))
	if err != nil {
		s.logger.Error("failed to check if rule exists", "err", err, "", rule)
	} else if !res.Msg.Exists {
		s.addRule(ctx, rule)
	}
}

func (s *IPRouteController) reconcileRoutes(ctx context.Context, cfg *RoutingConfig) {
	defer TrackDuration("reconcile_routes")()
	defer log.Profile(s.logger, "reconcile routes")()

	s.routesMu.Lock()
	defer s.routesMu.Unlock()

	definedRoutes := s.loadRoutes(ctx, cfg.Rule.Table)

	addRoute := func(route IPRoute) {
		_, defined := definedRoutes[route]
		delete(definedRoutes, route) // delete from set, to track unexpected routes later
		if !defined {
			s.addRoute(ctx, route)
		}
	}

	for _, route := range s.routes.Values() {
		records := removeExpiredRecords(s.dnsStore.LookupIP(route.Addr), cfg.RouteTimeout)
		if len(records) > 0 {
			addRoute(route)
		}
	}

	for iface, addresses := range cfg.Static {
		for _, addr := range addresses {
			addRoute(IPRoute{cfg.Rule.Table, iface, addr})
		}
	}

	for route := range definedRoutes {
		s.deleteRoute(ctx, route)
	}
}

func (s *IPRouteController) AddRoute(ctx context.Context, iface string, ip IPv4) {
	s.routesMu.Lock()
	defer s.routesMu.Unlock()
	route := IPRoute{s.tableId, iface, ip}
	if !s.routes.Has(route) {
		s.addRoute(ctx, route)
	}
}

func (s *IPRouteController) addRoute(ctx context.Context, route IPRoute) {
	defer TrackDuration("add_route")()

	_, err := s.networkService.AddRoute(ctx, connect.NewRequest(&agentv1.AddRouteReq{
		Route: mapToAgentRoute(route),
	}))
	if err != nil {
		s.logger.Error("failed to add route", "err", err, "", route)
	} else {
		s.logger.Info("route added", "", route)
		s.routes.Add(route)
	}
}

func (s *IPRouteController) deleteRoute(ctx context.Context, route IPRoute) {
	defer TrackDuration("delete_route")()

	_, err := s.networkService.DeleteRoute(ctx, connect.NewRequest(&agentv1.DeleteRouteReq{
		Route: mapToAgentRoute(route),
	}))
	if err != nil {
		s.logger.Error("failed to delete route", "err", err, "", route)
	} else {
		s.logger.Info("route deleted", "", route)
		s.routes.Remove(route)
	}
}

func (s *IPRouteController) addRule(ctx context.Context, rule IPRoutingRule) {
	defer TrackDuration("add_rule")()

	_, err := s.networkService.AddRule(ctx, connect.NewRequest(&agentv1.AddRuleReq{
		Rule: mapToAgentRule(rule),
	}))
	if err != nil {
		s.logger.Error("failed to add rule", "err", err, "", rule)
	} else {
		s.logger.Info("rule added", "", rule)
	}
}

func (s *IPRouteController) loadRoutes(ctx context.Context, tableId int) map[IPRoute]struct{} {
	defer TrackDuration("load_routes")()

	res, err := s.networkService.ListRoutes(ctx, connect.NewRequest(&agentv1.ListRoutesReq{Table: uint32(tableId)}))
	if err != nil {
		s.logger.Error("failed to load route table", "err", err, "table", tableId)
		return nil
	}

	routes := make(map[IPRoute]struct{}, len(res.Msg.Routes))
	for _, it := range res.Msg.Routes {
		addr, err := ParseIPv4(it.Address)
		if err != nil {
			s.logger.Warn("unexpected route address", "addr", it.Address)
			continue
		}
		route := IPRoute{int(it.Table), it.Iface, addr}
		routes[route] = struct{}{}
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
