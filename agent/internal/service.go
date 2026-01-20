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

	"github.com/mikhailv/keenetic-dns/agent/internal/keenetic"
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

func (s *networkService) HasRule(ctx context.Context, req *v1.HasRuleReq) (*v1.HasRuleResp, error) {
	cmd := exec.CommandContext(ctx, "ip", "rule", "list")
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to load rule list", "err", err, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}

	rule := req.Rule
	def := fmt.Sprintf("from all iif %s lookup %d", rule.Iif, rule.Table)

	resp := &v1.HasRuleResp{}
	for _, line := range parseOutputLines(res.Output) {
		// 2000:	from all iif br0 lookup 1000
		ss := strings.Split(line, ":")
		if len(ss) == 2 && strings.TrimSpace(ss[1]) == def {
			resp.Exists = true
			break
		}
	}
	return resp, nil
}

func (s *networkService) AddRule(ctx context.Context, req *v1.AddRuleReq) (*v1.AddRuleResp, error) {
	rule := req.Rule
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "rule", "add", "iif", rule.Iif, "table", fmt.Sprint(rule.Table), "priority", fmt.Sprint(rule.Priority))
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to add rule", "err", err, "", rule, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	s.logger.Info("rule added", "", rule)
	return &v1.AddRuleResp{}, nil
}

func (s *networkService) ListRoutes(ctx context.Context, req *v1.ListRoutesReq) (*v1.ListRoutesResp, error) {
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "list", "table", fmt.Sprint(req.Table))
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to load route table", "err", err, "table", req.Table, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	lines := parseOutputLines(res.Output)
	routes := make([]*v1.Route, 0, len(lines))
	for _, line := range lines {
		ss := strings.Split(line, " ")
		if len(ss) == 5 {
			// example: `209.85.233.100 dev ovpn_br0 scope link`
			routes = append(routes, &v1.Route{
				Table:   req.Table,
				Iface:   strings.Clone(ss[2]),
				Address: ss[0],
			})
		} else {
			s.logger.Warn("unexpected route output", "line", line)
		}
	}
	return &v1.ListRoutesResp{Routes: routes}, nil
}

func (s *networkService) AddRoute(ctx context.Context, req *v1.AddRouteReq) (*v1.AddRouteResp, error) {
	route := req.Route
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "add", "table", fmt.Sprint(route.Table), route.Address, "dev", route.Iface)
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to add route", "err", err, "", route, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	s.logger.Info("route added", "", route)
	return &v1.AddRouteResp{}, nil
}

func (s *networkService) DeleteRoute(ctx context.Context, req *v1.DeleteRouteReq) (*v1.DeleteRouteResp, error) {
	route := req.Route
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "del", "table", fmt.Sprint(route.Table), route.Address, "dev", route.Iface)
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to delete route", "err", err, "", route, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	s.logger.Info("route deleted", "", route)
	return &v1.DeleteRouteResp{}, nil
}

func (s *networkService) ListHosts(ctx context.Context, _ *v1.ListHostsReq) (*v1.ListHostsResp, error) {
	cmd := exec.CommandContext(ctx, "ndmc", "-c", "show ip hotspot")
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to get ip hotspots", "err", err, "output", res.ErrOutput)
		return nil, wrapError(err, res)
	}
	objs := keenetic.ParseOutput(res.Output)
	var resp v1.ListHostsResp
	resp.Hosts = make([]*v1.HostInfo, len(objs))
	for i, obj := range objs {
		resp.Hosts[i] = parseHostInfo(obj.GetObject("host"))
	}
	return &resp, nil
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

func parseOptionalObj[T any](obj keenetic.Object, parseFn func(obj keenetic.Object) *T) *T {
	if len(obj) == 0 {
		return nil
	}
	return parseFn(obj)
}

func parseHostInfo(obj keenetic.Object) *v1.HostInfo {
	return &v1.HostInfo{
		Mac:             obj.GetString("mac"),
		Via:             obj.GetString("via"),
		Ip:              obj.GetString("ip"),
		Hostname:        obj.GetString("hostname"),
		Name:            obj.GetString("name"),
		Registered:      obj.GetBool("registered"),
		Access:          obj.GetString("access"),
		Priority:        obj.GetInt("priority"),
		Active:          obj.GetBool("active"),
		RxBytes:         obj.GetInt("rxbytes"),
		TxBytes:         obj.GetInt("txbytes"),
		Link:            obj.GetString("link"),
		Uptime:          obj.GetInt("uptime"),
		FirstSeen:       obj.GetInt("first-seen"),
		LastSeen:        obj.GetInt("last-seen"),
		AutoNegotiation: obj.GetBool("auto-negotiation"),
		Speed:           obj.GetInt("speed"),
		Duplex:          obj.GetBool("duplex"),
		Port:            obj.GetInt("port"),
		SystemMode:      obj.GetString("system-mode"),
		HttpPort:        obj.GetInt("http-port"),
		HttpHost:        obj.GetString("http-host"),
		Region:          obj.GetString("region"),
		Description:     obj.GetString("description"),
		Firmware:        obj.GetString("firmware"),
		Interface: parseOptionalObj(obj.GetObject("interface"), func(obj keenetic.Object) *v1.HostInterface {
			return &v1.HostInterface{
				Id:          obj.GetString("id"),
				Name:        obj.GetString("name"),
				Description: obj.GetString("description"),
			}
		}),
		Dhcp: parseOptionalObj(obj.GetObject("dhcp"), func(obj keenetic.Object) *v1.HostDHCP {
			return &v1.HostDHCP{
				Static: obj.GetBool("static"),
			}
		}),
		Mws: parseOptionalObj(obj.GetObject("mws"), func(obj keenetic.Object) *v1.HostMWS {
			return &v1.HostMWS{
				Cid:           obj.GetString("cid"),
				Ap:            obj.GetString("ap"),
				Psm:           obj.GetBool("psm"),
				Mld:           obj.GetBool("mld"),
				Authenticated: obj.GetBool("authenticated"),
				TxRate:        obj.GetInt("txrate"),
				Uptime:        obj.GetInt("uptime"),
				Rssi:          obj.GetInt("rssi"),
				Mcs:           obj.GetInt("mcs"),
				Security:      obj.GetString("security"),
			}
		}),
	}
}
