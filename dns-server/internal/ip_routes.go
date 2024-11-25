package internal

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"

	"connectrpc.com/connect"

	"github.com/mikhailv/keenetic-dns/agent"
	agentv1 "github.com/mikhailv/keenetic-dns/agent/rpc/v1"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type ipRouteJob struct {
	IPRoute
	done chan struct{}
}

type IPRouteController struct {
	cfg               RoutingConfig
	logger            *slog.Logger
	dnsStore          *DNSStore
	networkService    agent.NetworkServiceClient
	routes            *util.SyncSet[IPRoute]
	addQueue          chan ipRouteJob
	deleteQueue       chan IPRoute
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
	return &IPRouteController{
		cfg:               cfg,
		logger:            logger,
		dnsStore:          dnsStore,
		networkService:    networkService,
		routes:            util.NewSyncSet[IPRoute](),
		addQueue:          make(chan ipRouteJob),
		deleteQueue:       make(chan IPRoute),
		reconcileInterval: reconcileInterval,
		reconcileTimeout:  reconcileTimeout,
	}
}

func (s *IPRouteController) LookupHost(host string) (iface string) {
	return s.cfg.LookupHost(host)
}

func (s *IPRouteController) Routes() []IPRouteDNS {
	res := make([]IPRouteDNS, 0, s.routes.Size())
	for _, route := range s.routes.Values() {
		records := removeExpiredRecords(s.dnsStore.LookupIP(route.Addr), s.cfg.RouteTimeout)
		slices.SortFunc(records, func(a, b DNSRecord) int {
			return cmp.Compare(a.Domain, b.Domain)
		})
		res = append(res, IPRouteDNS{route, records})
	}
	return res
}

func (s *IPRouteController) Start(ctx context.Context) {
	s.init()
	go s.startQueueProcessor(ctx)
	s.reconcile(ctx)
	go util.RunPeriodically(ctx, s.reconcileInterval, s.reconcile)
}

func (s *IPRouteController) init() {
	for _, rec := range s.dnsStore.Records() {
		if iface := s.cfg.LookupHost(rec.Domain); iface != "" {
			s.routes.Add(IPRoute{s.cfg.Table, iface, rec.IP})
		}
	}
}

func (s *IPRouteController) startQueueProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.addQueue:
			s.addRoute(ctx, job.IPRoute)
			close(job.done)
		default:
			select {
			case <-ctx.Done():
				return
			case job := <-s.addQueue:
				s.addRoute(ctx, job.IPRoute)
				close(job.done)
			case route := <-s.deleteQueue:
				s.deleteRoute(ctx, route)
			}
		}
	}
}

func (s *IPRouteController) reconcile(ctx context.Context) {
	s.dnsStore.RemoveExpired(s.cfg.RouteTimeout)
	s.doReconcile(ctx, s.reconcileRoutes)
	s.doReconcile(ctx, s.reconcileRules)
}

func (s *IPRouteController) doReconcile(ctx context.Context, fn func(context.Context)) {
	ctx, cancel := context.WithTimeout(ctx, s.reconcileTimeout)
	defer cancel()
	fn(ctx)
}

func (s *IPRouteController) reconcileRules(ctx context.Context) {
	defer TrackDuration("reconcile_rules")()

	s.logger.Info("reconcile rules")

	rule := s.cfg.RoutingRule()
	res, err := s.networkService.HasRule(ctx, connect.NewRequest(&agentv1.HasRuleReq{
		Rule: mapToAgentRule(rule),
	}))
	if err != nil {
		s.logger.Error("failed to check if rule exists", "err", err, "", rule)
	} else if !res.Msg.Exists {
		s.addRule(ctx, rule)
	}
}

func (s *IPRouteController) reconcileRoutes(ctx context.Context) {
	defer TrackDuration("reconcile_routes")()

	s.logger.Info("reconcile routes")

	definedRoutes := s.loadRoutes(ctx, s.cfg.Table)

	addRoute := func(route IPRoute) {
		_, defined := definedRoutes[route]
		delete(definedRoutes, route) // delete from set, to track unexpected routes later
		if !defined {
			s.enqueueAddRoute(ctx, route, false)
		}
	}

	for _, route := range s.routes.Values() {
		records := removeExpiredRecords(s.dnsStore.LookupIP(route.Addr), s.cfg.RouteTimeout)
		if len(records) > 0 {
			addRoute(route)
		}
	}

	for iface, addresses := range s.cfg.Static {
		for _, addr := range addresses {
			route := IPRoute{s.cfg.Table, iface, addr}
			s.routes.Add(route)
			addRoute(route)
		}
	}

	for route := range definedRoutes {
		s.routes.Remove(route)
		s.enqueueDeleteRoute(ctx, route)
	}
}

func (s *IPRouteController) AddRoute(ctx context.Context, iface string, ip IPv4) {
	route := IPRoute{s.cfg.Table, iface, ip}
	if s.routes.Add(route) {
		s.enqueueAddRoute(ctx, route, true)
	}
}

func (s *IPRouteController) enqueueAddRoute(ctx context.Context, route IPRoute, block bool) {
	job := ipRouteJob{route, make(chan struct{})}
	select {
	case <-ctx.Done():
		s.logger.Error("failed to add route", "err", context.Cause(ctx), "", route)
		return
	case s.addQueue <- job:
		if block {
			select {
			case <-ctx.Done():
				s.logger.Error("failed to add route", "err", context.Cause(ctx), "", route)
				return
			case <-job.done:
			}
		}
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

func (s *IPRouteController) enqueueDeleteRoute(ctx context.Context, route IPRoute) {
	select {
	case <-ctx.Done():
		s.logger.Error("failed to delete route", "err", context.Cause(ctx), "", route)
		return
	case s.deleteQueue <- route:
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
