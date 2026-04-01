package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mikhailv/keenetic-dns/agent/internal/api"
	"github.com/mikhailv/keenetic-dns/agent/internal/keenetic"
)

func NewNetworkService(logger *slog.Logger) api.StrictServerInterface {
	return &networkService{logger}
}

var _ api.StrictServerInterface = &networkService{}

type networkService struct {
	logger *slog.Logger
}

func (s *networkService) HasRule(ctx context.Context, req api.HasRuleRequestObject) (api.HasRuleResponseObject, error) {
	cmd := exec.CommandContext(ctx, "ip", "rule", "list")
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to load rule list", "err", err, "output", res.ErrOutput)
		return api.HasRule500JSONResponse(makeError(err, res)), nil
	}

	def := fmt.Sprintf("from all iif %s lookup %d", req.Params.Iif, req.Params.Table)

	for _, line := range parseOutputLines(res.Output) {
		// 2000:	from all iif br0 lookup 1000
		ss := strings.Split(line, ":")
		if len(ss) == 2 && strings.TrimSpace(ss[1]) == def {
			return api.HasRule200Response{}, nil
		}
	}
	return api.HasRule404Response{}, nil
}

func (s *networkService) AddRule(ctx context.Context, req api.AddRuleRequestObject) (api.AddRuleResponseObject, error) {
	rule := req.Body
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "rule", "add", "iif", rule.Iif, "table", u32ToStr(rule.Table), "priority", u32ToStr(rule.Priority))
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to add rule", "err", err, "rule", rule, "output", res.ErrOutput)
		return api.AddRule500JSONResponse(makeError(err, res)), nil
	}
	s.logger.Info("rule added", "rule", rule)
	return api.AddRule204Response{}, nil
}

func (s *networkService) ListRoutes(ctx context.Context, req api.ListRoutesRequestObject) (api.ListRoutesResponseObject, error) {
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "list", "table", u32ToStr(req.Params.Table))
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to load route table", "err", err, "table", req.Params.Table, "output", res.ErrOutput)
		return api.ListRoutes500JSONResponse(makeError(err, res)), nil
	}
	lines := parseOutputLines(res.Output)
	routes := make([]api.Route, 0, len(lines))
	for _, line := range lines {
		ss := strings.Split(line, " ")
		if len(ss) == 5 {
			// example: `209.85.233.100 dev ovpn_br0 scope link`
			routes = append(routes, api.Route{
				Table:   req.Params.Table,
				Iface:   strings.Clone(ss[2]),
				Address: ss[0],
			})
		} else {
			s.logger.Warn("unexpected route output", "line", line)
		}
	}
	return api.ListRoutes200JSONResponse(routes), nil
}

func (s *networkService) AddRoute(ctx context.Context, req api.AddRouteRequestObject) (api.AddRouteResponseObject, error) {
	route := req.Body
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "add", "table", u32ToStr(route.Table), route.Address, "dev", route.Iface)
	res, err := s.runCmd(cmd)
	if err != nil && !strings.Contains(res.ErrOutput, "ip: RTNETLINK answers: File exists") {
		s.logger.Error("failed to add route", "err", err, "route", route, "output", res.ErrOutput)
		return api.AddRoute500JSONResponse(makeError(err, res)), nil
	}
	s.logger.Info("route added", "route", route)
	return api.AddRoute204Response{}, nil
}

func (s *networkService) DeleteRoute(ctx context.Context, req api.DeleteRouteRequestObject) (api.DeleteRouteResponseObject, error) {
	route := req.Body
	//nolint:gosec // all fine
	cmd := exec.CommandContext(ctx, "ip", "route", "del", "table", u32ToStr(route.Table), route.Address, "dev", route.Iface)
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to delete route", "err", err, "route", route, "output", res.ErrOutput)
		return api.DeleteRoute500JSONResponse(makeError(err, res)), nil
	}
	s.logger.Info("route deleted", "route", route)
	return api.DeleteRoute204Response{}, nil
}

func (s *networkService) ListHosts(ctx context.Context, _ api.ListHostsRequestObject) (api.ListHostsResponseObject, error) {
	cmd := exec.CommandContext(ctx, "ndmc", "-c", "show device-list")
	res, err := s.runCmd(cmd)
	if err != nil {
		s.logger.Error("failed to get device list", "err", err, "output", res.ErrOutput)
		return api.ListHosts500JSONResponse(makeError(err, res)), nil
	}
	objs, err := keenetic.ParseOutput(res.Output)
	if err != nil {
		s.logger.Error("failed to parse ndmc command output", "err", err)
		return api.ListHosts500JSONResponse{Error: err.Error()}, nil
	}
	hosts := make([]api.HostInfo, len(objs))
	for i, obj := range objs {
		hosts[i] = parseHostInfo(obj.GetObject("host"))
	}
	return api.ListHosts200JSONResponse(hosts), nil
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

func makeError(err error, r cmdRunResult) api.Error {
	resp := api.Error{Error: err.Error()}
	if r.ExitCode != 0 {
		exitCode := int32(r.ExitCode)
		resp.ExitCode = &exitCode
	}
	if r.ErrOutput != "" {
		resp.Output = &r.ErrOutput
	}
	return resp
}

func parseOptionalObj[T any](obj keenetic.Object, parseFn func(obj keenetic.Object) *T) *T {
	if len(obj) == 0 {
		return nil
	}
	return parseFn(obj)
}

func parseHostInfo(obj keenetic.Object) api.HostInfo {
	return api.HostInfo{
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
		Interface: parseOptionalObj(obj.GetObject("interface"), func(obj keenetic.Object) *api.HostInterface {
			return &api.HostInterface{
				Id:          obj.GetString("id"),
				Name:        obj.GetString("name"),
				Description: obj.GetString("description"),
			}
		}),
		Dhcp: parseOptionalObj(obj.GetObject("dhcp"), func(obj keenetic.Object) *api.HostDHCP {
			return &api.HostDHCP{
				Expires: obj.GetInt("expires"),
			}
		}),
		Mws: parseOptionalObj(obj.GetObject("mws"), func(obj keenetic.Object) *api.HostMWS {
			return &api.HostMWS{
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

func u32ToStr(v uint32) string {
	return strconv.FormatUint(uint64(v), 10)
}
