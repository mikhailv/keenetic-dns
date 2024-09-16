package internal

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

type IPRouteController struct {
	cfg               RoutingConfig
	logger            *slog.Logger
	dnsStore          *DNSStore
	routes            *util.SyncSet[IPRoute]
	addQueue          chan IPRoute
	deleteQueue       chan IPRoute
	reconcileInterval time.Duration
	reconcileTimeout  time.Duration
}

func NewIPRouteController(
	cfg RoutingConfig,
	logger *slog.Logger,
	dnsStore *DNSStore,
	reconcileInterval time.Duration,
	reconcileTimeout time.Duration,
) *IPRouteController {
	return &IPRouteController{
		cfg:               cfg,
		logger:            logger,
		dnsStore:          dnsStore,
		routes:            util.NewSyncSet[IPRoute](),
		addQueue:          make(chan IPRoute),
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
			s.routes.Add(IPRoute{rec.IP, iface})
		}
	}
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

	output, errOutput, err := s.runCmd(exec.CommandContext(ctx, "ip", "rule", "list"))
	if err != nil {
		s.logger.Error("failed to load rule list", "err", err, "output", errOutput)
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

	definedRoutes := s.loadRoutes(ctx)

	addRoute := func(route IPRoute) {
		_, defined := definedRoutes[route]
		delete(definedRoutes, route) // delete from set, to track unexpected routes later
		if !defined {
			s.enqueueAddRoute(ctx, route)
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
			route := IPRoute{addr, iface}
			s.routes.Add(route)
			addRoute(route)
		}
	}

	for route := range definedRoutes {
		s.routes.Remove(route)
		s.enqueueDeleteRoute(ctx, route)
	}
}

func (s *IPRouteController) AddRoute(ctx context.Context, route IPRoute) {
	if s.routes.Add(route) {
		s.enqueueAddRoute(ctx, route)
	}
}

func (s *IPRouteController) enqueueAddRoute(ctx context.Context, route IPRoute) {
	select {
	case <-ctx.Done():
		s.logger.Error("failed to add route", "err", context.Cause(ctx), "", route, "table", s.cfg.Table)
		return
	case s.addQueue <- route:
	}
}

func (s *IPRouteController) addRoute(ctx context.Context, route IPRoute) {
	defer TrackDuration("add_route")()

	//nolint:gosec // all fine
	_, errOutput, err := s.runCmd(exec.CommandContext(ctx, "ip", "route", "add", "table", strconv.Itoa(s.cfg.Table), route.Addr.String(), "dev", route.Iface))
	if err != nil {
		s.logger.Error("failed to add route", "err", err, "", route, "table", s.cfg.Table, "output", errOutput)
	} else {
		s.logger.Info("route added", "", route, "table", s.cfg.Table)
		s.routes.Add(route)
	}
}

func (s *IPRouteController) enqueueDeleteRoute(ctx context.Context, route IPRoute) {
	select {
	case <-ctx.Done():
		s.logger.Error("failed to delete route", "err", context.Cause(ctx), "", route, "table", s.cfg.Table)
		return
	case s.deleteQueue <- route:
	}
}

func (s *IPRouteController) deleteRoute(ctx context.Context, route IPRoute) {
	defer TrackDuration("delete_route")()

	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "del", "table", strconv.Itoa(s.cfg.Table), route.Addr.String(), "dev", route.Iface)
	if err := cmd.Run(); err != nil {
		s.logger.Error("failed to delete route", "err", err, "", route, "table", s.cfg.Table)
	} else {
		s.logger.Info("route deleted", "", route, "table", s.cfg.Table)
	}
}

func (s *IPRouteController) addRule(ctx context.Context, rule IPRoutingRule) {
	defer TrackDuration("add_rule")()

	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "rule", "add", "iif", rule.Iif, "table", strconv.Itoa(rule.TableID), "priority", strconv.Itoa(rule.Priority))
	if err := cmd.Run(); err != nil {
		s.logger.Error("failed to add rule", "err", err, "rule", rule)
	} else {
		s.logger.Info("rule added", "rule", rule)
	}
}

func (s *IPRouteController) loadRoutes(ctx context.Context) map[IPRoute]struct{} {
	defer TrackDuration("load_routes")()

	//nolint:gosec // all fine
	output, errOutput, err := s.runCmd(exec.CommandContext(ctx, "ip", "route", "list", "table", strconv.Itoa(s.cfg.Table)))
	if err != nil {
		s.logger.Error("failed to load route table", "err", err, "table", s.cfg.Table, "output", errOutput)
		return nil
	}

	routes := make(map[IPRoute]struct{}, s.routes.Size())
	for _, line := range parseOutputLines(output) {
		ss := strings.Split(line, " ")
		if len(ss) == 5 {
			// example: `209.85.233.100 dev ovpn_br0 scope link`
			route := IPRoute{Iface: strings.Clone(ss[2])}
			route.Addr, err = ParseIPv4(ss[0])
			if err != nil {
				s.logger.Warn("unexpected route address", "line", line, "err", err)
			} else {
				routes[route] = struct{}{}
			}
		} else {
			s.logger.Warn("unexpected route output", "line", line)
		}
	}
	return routes
}

func (s *IPRouteController) runCmd(cmd *exec.Cmd) (stdout string, stderr string, err error) {
	cmdArgs := strings.Join(cmd.Args, " ")
	s.logger.Debug("command start", slog.String("cmd", cmdArgs))
	startTime := time.Now()

	output, err := cmd.Output()

	var exitCode int
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
		stderr = string(exitErr.Stderr)
	}

	s.logger.Debug("command exit", slog.String("cmd", cmdArgs), slog.Int("exit_code", exitCode), slog.Duration("duration", time.Since(startTime)))
	return string(output), stderr, err
}

func parseOutputLines(output string) []string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return slices.DeleteFunc(lines, func(s string) bool { return s == "" })
}
