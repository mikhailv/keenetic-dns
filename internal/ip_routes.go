package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type IPRouteController struct {
	mu                sync.Mutex
	logger            *slog.Logger
	cfg               RoutingConfig
	dnsStore          *DNSStore
	routes            map[IPRouteKey]IPRoute
	addQueue          chan IPRoute
	deleteQueue       chan IPRoute
	reconcileInterval time.Duration
}

func NewIPRouteController(
	cfg RoutingConfig,
	logger *slog.Logger,
	dnsStore *DNSStore,
	reconcileInterval time.Duration,
) *IPRouteController {
	return &IPRouteController{
		cfg:               cfg,
		logger:            logger,
		dnsStore:          dnsStore,
		routes:            map[IPRouteKey]IPRoute{},
		addQueue:          make(chan IPRoute),
		deleteQueue:       make(chan IPRoute),
		reconcileInterval: reconcileInterval,
	}
}

func (s *IPRouteController) Start(ctx context.Context) {
	go s.startQueueProcessor(ctx)
	s.reconcile(ctx)
	go RunPeriodically(ctx, s.reconcileInterval, s.reconcileRoutes)
}

func (s *IPRouteController) startQueueProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case route := <-s.addQueue:
			s.addRoute(ctx, route)
		default:
			select {
			case <-ctx.Done():
				return
			case route := <-s.addQueue:
				s.addRoute(ctx, route)
			case route := <-s.deleteQueue:
				s.deleteRoute(ctx, route)
			}
		}
	}
}

func (s *IPRouteController) reconcile(ctx context.Context) {
	s.reconcileRoutes(ctx)
	s.reconcileRules(ctx)
}

func (s *IPRouteController) reconcileRules(ctx context.Context) {
	defer TrackDuration("reconcile_rules")()

	s.logger.Info("reconcile rules")

	output, errOutput, err := runCmd(exec.CommandContext(ctx, "ip", "rule", "list"))
	if err != nil {
		s.logger.Error("failed to load rule list", slog.Any("err", err), slog.String("output", errOutput))
		return
	}

	definedRules := map[string]bool{}
	for _, line := range parseOutputLines(output) {
		ss := strings.Split(line, ":")
		if len(ss) == 2 {
			definedRules[strings.TrimSpace(ss[1])] = true
		}
	}

	for _, rule := range []IPRoutingRule{s.cfg.RoutingRule()} {
		def := fmt.Sprintf("from all iif %s lookup %d", rule.Iif, rule.TableID)
		if !definedRules[def] {
			s.addRule(ctx, rule)
		}
	}
}

func (s *IPRouteController) reconcileRoutes(ctx context.Context) {
	defer TrackDuration("reconcile_routes")()

	s.logger.Info("reconcile routes")

	defined := s.loadRoutes(ctx)
	actual := map[IPRouteKey]IPRoute{}
	for _, rec := range s.dnsStore.Records() {
		if iface, ok := s.cfg.LookupHost(rec.Domain); ok {
			route := IPRoute{rec, iface}
			actual[route.Key()] = route
		} else if rec.Expired() {
			s.dnsStore.Remove(rec.DNSRecordKey)
		}
	}

	for key, route := range actual {
		expired := route.Expired(s.cfg.RouteTimeout)
		if _, ok := defined[key]; ok {
			if expired {
				s.enqueueDeleteRoute(ctx, route)
			} else {
				s.mu.Lock()
				s.routes[key] = route
				s.mu.Unlock()
			}
			delete(defined, key)
		} else {
			if expired {
				s.dnsStore.Remove(route.DNSRecordKey)
			} else {
				s.enqueueAddRoute(ctx, route)
			}
		}
	}
	for _, route := range defined {
		s.enqueueDeleteRoute(ctx, route)
	}
}

func (s *IPRouteController) AddRoute(ctx context.Context, rec DNSRecord, iface string) {
	s.enqueueAddRoute(ctx, IPRoute{rec, iface})
}

func (s *IPRouteController) enqueueAddRoute(ctx context.Context, route IPRoute) {
	select {
	case <-ctx.Done():
		return
	case s.addQueue <- route:
	}
}

func (s *IPRouteController) addRoute(ctx context.Context, route IPRoute) {
	defer TrackDuration("add_route")()

	s.dnsStore.Add(route.DNSRecord)

	s.mu.Lock()
	defer s.mu.Unlock()

	key := route.Key()
	if _, ok := s.routes[key]; ok { // route already defined
		s.logger.Info("route updated", slog.Any("", route))
		s.routes[key] = route // update route info (ttl, domain)
		return
	}

	//nolint:gosec // all fine
	_, errOutput, err := runCmd(exec.CommandContext(ctx, "ip", "route", "add", "table", strconv.Itoa(s.cfg.Table), route.IP.String(), "dev", route.Iface))
	if err != nil {
		s.logger.Error("failed to add route", slog.Any("err", err), slog.Any("", route), slog.Int("table", s.cfg.Table), slog.String("output", errOutput))
	} else {
		s.logger.Info("route added", slog.Any("", route), slog.Int("table", s.cfg.Table))
		s.routes[key] = route
	}
}

func (s *IPRouteController) enqueueDeleteRoute(ctx context.Context, route IPRoute) {
	select {
	case <-ctx.Done():
		return
	case s.deleteQueue <- route:
	}
}

func (s *IPRouteController) deleteRoute(ctx context.Context, route IPRoute) {
	defer TrackDuration("delete_route")()

	s.dnsStore.Remove(route.DNSRecordKey)

	s.mu.Lock()
	defer s.mu.Unlock()

	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "del", "table", strconv.Itoa(s.cfg.Table), route.IP.String(), "dev", route.Iface)
	if err := cmd.Run(); err != nil {
		s.logger.Error("failed to delete route", slog.Any("err", err), slog.Any("", route), slog.Int("table", s.cfg.Table))
	} else {
		s.logger.Info("route deleted", slog.Any("", route), slog.Int("table", s.cfg.Table))
		delete(s.routes, route.Key())
	}
}

func (s *IPRouteController) addRule(ctx context.Context, rule IPRoutingRule) {
	defer TrackDuration("add_rule")()

	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "rule", "add", "iff", rule.Iif, "table", strconv.Itoa(rule.TableID), "priority", strconv.Itoa(rule.Priority))
	if err := cmd.Run(); err != nil {
		s.logger.Error("failed to add rule", slog.Any("err", err), slog.Any("rule", rule))
	} else {
		s.logger.Info("rule added", slog.Any("rule", rule))
	}
}

func (s *IPRouteController) loadRoutes(ctx context.Context) map[IPRouteKey]IPRoute {
	defer TrackDuration("load_routes")()

	//nolint:gosec // all fine
	output, errOutput, err := runCmd(exec.CommandContext(ctx, "ip", "route", "list", "table", strconv.Itoa(s.cfg.Table)))
	if err != nil {
		s.logger.Error("failed to load route table", slog.Any("err", err), slog.Int("table", s.cfg.Table), slog.String("output", errOutput))
		return nil
	}

	routes := map[IPRouteKey]IPRoute{}
	for _, line := range parseOutputLines(output) {
		ss := strings.Split(line, " ")
		if len(ss) == 5 {
			// example: `209.85.233.100 dev ovpn_br0 scope link`
			ip := NewIPv4(net.ParseIP(ss[0]))
			iface := ss[2]
			route := IPRoute{NewDNSRecord("", ip, s.cfg.RouteTimeout), iface}
			routes[route.Key()] = route
		} else {
			s.logger.Warn("unexpected route output", slog.String("line", line))
		}
	}
	return routes
}

func parseOutputLines(output string) []string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return slices.DeleteFunc(lines, func(s string) bool { return s == "" })
}

func runCmd(cmd *exec.Cmd) (stdout string, stderr string, err error) {
	output, err := cmd.Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(output), string(exitErr.Stderr), err
	}
	return string(output), "", err
}
