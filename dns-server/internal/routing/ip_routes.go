package routing

import (
	"cmp"
	"context"
	"log/slog"
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
	routes         util.SyncSet[IPRoute]
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
	res := make([]IPRouteDNS, 0, s.routes.Size())
	for _, route := range s.routes.Values() {
		records := s.dnsStore.LookupIP(route.Addr)
		slices.SortFunc(records, func(a, b types.DNSRecord) int {
			return cmp.Compare(a.Domain, b.Domain)
		})
		res = append(res, IPRouteDNS{route, records})
	}
	return res
}

func (s *IPRouteController) Start(ctx context.Context) {
	s.cfg.Listen(func() { s.onConfigUpdated(ctx) })
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

	definedRoutes := s.loadRoutes(ctx, cfg.Rule.Table)

	actual := make(map[IPRoute]string, s.routes.Size())
	obsolete := make(map[IPRoute]string, 10)

	for _, addr := range cfg.Static {
		actual[s.makeRoute(cfg, addr)] = "static"
	}

	for rec := range s.dnsStore.RecordIterator() {
		route := s.makeRoute(cfg, rec.IP)
		if actual[route] != "" {
			continue
		}
		if pattern := cfg.LookupHost(rec.Domain); pattern != "" {
			actual[route] = "match: " + pattern
			delete(obsolete, route)
		} else {
			obsolete[route] = "no match"
		}
	}

	for route := range definedRoutes {
		if actual[route] == "" {
			obsolete[route] = "expired"
		}
	}

	for route := range s.routes.Iterator() {
		if actual[route] == "" && obsolete[route] == "" {
			obsolete[route] = "obsolete"
		}
	}

	added := 0
	deleted := 0

	for route, reason := range actual {
		if definedRoutes.Has(route) {
			s.routes.Add(route)
		} else if s.addRoute(ctx, route, reason) {
			s.routes.Add(route)
			added++
		}
	}

	for route, reason := range obsolete {
		if !definedRoutes.Has(route) {
			s.routes.Remove(route)
		} else if s.deleteRoute(ctx, route, reason) {
			s.routes.Remove(route)
			deleted++
		}
	}

	s.logger.Info("routes updated", "added", added, "deleted", deleted, "total", s.routes.Size())
}

func (s *IPRouteController) AddRoute(ctx context.Context, ip types.IPv4, reason string) bool {
	route := s.makeRoute(s.cfg.Get(), ip)
	if !s.routes.Has(route) && s.addRoute(ctx, route, reason) {
		s.routes.Add(route)
		return true
	}
	return false
}

func (s *IPRouteController) addRoute(ctx context.Context, route IPRoute, reason string) bool {
	defer metrics.TrackDuration("add_route")()

	_, err := s.networkService.AddRoute(ctx, &agentv1.AddRouteReq{
		Route: mapToAgentRoute(route),
	})
	if err != nil {
		s.logger.Error("failed to add route", "err", err, "", route)
		return false
	}
	s.logger.Info("route added", "", route, "reason", reason)
	return true
}

func (s *IPRouteController) deleteRoute(ctx context.Context, route IPRoute, reason string) bool {
	defer metrics.TrackDuration("delete_route")()

	_, err := s.networkService.DeleteRoute(ctx, &agentv1.DeleteRouteReq{
		Route: mapToAgentRoute(route),
	})
	if err != nil {
		s.logger.Error("failed to delete route", "err", err, "", route)
		return false
	}
	s.logger.Info("route deleted", "", route, "reason", reason)
	return true
}

func (s *IPRouteController) addRule(ctx context.Context, rule IPRoutingRule) bool {
	defer metrics.TrackDuration("add_rule")()

	_, err := s.networkService.AddRule(ctx, &agentv1.AddRuleReq{
		Rule: mapToAgentRule(rule),
	})
	if err != nil {
		s.logger.Error("failed to add rule", "err", err, "", rule)
		return false
	}
	s.logger.Info("rule added", "", rule)
	return true
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
