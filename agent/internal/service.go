package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/mikhailv/keenetic-dns/agent/rpc/v1"
	"github.com/mikhailv/keenetic-dns/agent/rpc/v1/agentv1connect"
)

func NewNetworkService(logger *slog.Logger) agentv1connect.NetworkServiceHandler {
	return &networkService{logger}
}

var _ agentv1connect.NetworkServiceHandler = &networkService{}

type networkService struct {
	logger *slog.Logger
}

func (s *networkService) HasRule(ctx context.Context, req *connect.Request[v1.HasRuleReq]) (*connect.Response[v1.HasRuleResp], error) {
	cmd := exec.CommandContext(ctx, "ip", "rule", "list")
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to load rule list", "err", err, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}

	rule := req.Msg.Rule
	def := fmt.Sprintf("from all iif %s lookup %d", rule.Iif, rule.Table)

	resp := connect.NewResponse(&v1.HasRuleResp{})
	for _, line := range parseOutputLines(res.Output) {
		// 2000:	from all iif br0 lookup 1000
		ss := strings.Split(line, ":")
		if len(ss) == 2 && strings.TrimSpace(ss[1]) == def {
			resp.Msg.Exists = true
			break
		}
	}
	return resp, nil
}

func (s *networkService) AddRule(ctx context.Context, req *connect.Request[v1.AddRuleReq]) (*connect.Response[v1.AddRuleResp], error) {
	rule := req.Msg.Rule
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "rule", "add", "iif", rule.Iif, "table", fmt.Sprint(rule.Table), "priority", fmt.Sprint(rule.Priority))
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to add rule", "err", err, "", rule, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	s.logger.Info("rule added", "", rule)
	return connect.NewResponse(&v1.AddRuleResp{}), nil
}

func (s *networkService) ListRoutes(ctx context.Context, req *connect.Request[v1.ListRoutesReq]) (*connect.Response[v1.ListRoutesResp], error) {
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "list", "table", fmt.Sprint(req.Msg.Table))
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to load route table", "err", err, "table", req.Msg.Table, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	lines := parseOutputLines(res.Output)
	routes := make([]*v1.Route, 0, len(lines))
	for _, line := range lines {
		ss := strings.Split(line, " ")
		if len(ss) == 5 {
			// example: `209.85.233.100 dev ovpn_br0 scope link`
			routes = append(routes, &v1.Route{
				Table:   req.Msg.Table,
				Iface:   strings.Clone(ss[2]),
				Address: ss[0],
			})
		} else {
			s.logger.Warn("unexpected route output", "line", line)
		}
	}
	return connect.NewResponse(&v1.ListRoutesResp{Routes: routes}), nil
}

func (s *networkService) AddRoute(ctx context.Context, req *connect.Request[v1.AddRouteReq]) (*connect.Response[v1.AddRouteResp], error) {
	route := req.Msg.Route
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "add", "table", fmt.Sprint(route.Table), route.Address, "dev", route.Iface)
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to add route", "err", err, "", route, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	s.logger.Info("route added", "", route)
	return connect.NewResponse(&v1.AddRouteResp{}), nil
}

func (s *networkService) DeleteRoute(ctx context.Context, req *connect.Request[v1.DeleteRouteReq]) (*connect.Response[v1.DeleteRouteResp], error) {
	route := req.Msg.Route
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "del", "table", fmt.Sprint(route.Table), route.Address, "dev", route.Iface)
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to delete route", "err", err, "", route, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	s.logger.Info("route deleted", "", route)
	return connect.NewResponse(&v1.DeleteRouteResp{}), nil
}

type cmdRunResult struct {
	Output    string
	ErrOutput string
	ExitCode  int
}

func (s *networkService) runCmd(cmd *exec.Cmd) (cmdRunResult, error) {
	cmdArgs := strings.Join(cmd.Args, " ")
	s.logger.Debug("command started", slog.String("cmd", cmdArgs))
	startTime := time.Now()

	output, err := cmd.Output()

	res := cmdRunResult{
		Output: string(output),
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		res.ErrOutput = string(exitErr.Stderr)
	}

	s.logger.Info("command executed", slog.String("cmd", cmdArgs), slog.Int("exit_code", res.ExitCode), slog.Duration("duration", time.Since(startTime)))
	return res, err
}

func parseOutputLines(output string) []string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return slices.DeleteFunc(lines, func(s string) bool { return s == "" })
}

func wrapError(err error, r cmdRunResult) error {
	if r.ErrOutput == "" && r.ExitCode == 0 {
		return err
	}
	errInfo := v1.CmdErrorInfo{
		ExitCode: int32(r.ExitCode),
		Output:   r.ErrOutput,
	}
	connErr := connect.NewError(connect.CodeInternal, err)
	if detail, _ := connect.NewErrorDetail(&errInfo); detail != nil {
		connErr.AddDetail(detail)
	}
	return connErr
}
