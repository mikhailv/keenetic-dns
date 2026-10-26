package handlers

import (
	"context"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/resolvestats"
)

type ResolveRecorder interface {
	RecordResponse(origin string, resp *dns.Msg)
}

func NewResolveStatsHandler(handler dnssvc.Handler, recorder ResolveRecorder) dnssvc.Handler {
	return resolveStatsHandler{handler: handler, recorder: recorder}
}

var _ dnssvc.Handler = resolveStatsHandler{}

type resolveStatsHandler struct {
	handler  dnssvc.Handler
	recorder ResolveRecorder
}

func (s resolveStatsHandler) Handle(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	resp, err := s.handler.Handle(ctx, req)
	if err == nil {
		s.recorder.RecordResponse(resolvestats.OriginClient, resp)
	}
	return resp, err
}
