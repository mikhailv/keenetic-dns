package handlers

import (
	"context"
	"strings"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
)

const (
	LabelBlocked = "blocked"
	LabelRouted  = "routed"
)

func NewQueryStatsHandler(handler dnssvc.Handler, recorder StatsRecorder) dnssvc.Handler {
	return queryStatsHandler{handler: handler, recorder: recorder}
}

var _ dnssvc.Handler = queryStatsHandler{}

type queryStatsHandler struct {
	handler  dnssvc.Handler
	recorder StatsRecorder
}

func (s queryStatsHandler) Handle(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if !dnssvc.HasSingleQuestion(req) {
		return s.handler.Handle(ctx, req)
	}
	ctx = dnssvc.WithQueryStatus(ctx)

	resp, err := s.handler.Handle(ctx, req)

	question := req.Question[0]
	s.recorder.Record(
		ctxutil.GetDNSQueryClientIP(ctx),
		strings.TrimSuffix(question.Name, "."),
		dns.TypeToString[question.Qtype],
		label(dnssvc.GetQueryStatus(ctx)),
	)
	return resp, err
}

func label(status dnssvc.QueryStatus) string {
	switch {
	case status.Blocked:
		return LabelBlocked
	case status.Routed:
		return LabelRouted
	default:
		return ""
	}
}
