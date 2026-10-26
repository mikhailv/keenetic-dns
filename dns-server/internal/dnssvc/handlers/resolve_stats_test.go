package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/resolvestats"
)

type recordedResponse struct {
	origin string
	resp   *dns.Msg
}

type resolveRecorderStub struct {
	recorded []recordedResponse
}

func (s *resolveRecorderStub) RecordResponse(origin string, resp *dns.Msg) {
	s.recorded = append(s.recorded, recordedResponse{origin: origin, resp: resp})
}

type stubHandler struct {
	resp *dns.Msg
	err  error
}

func (s stubHandler) Handle(context.Context, *dns.Msg) (*dns.Msg, error) {
	return s.resp, s.err
}

var _ dnssvc.Handler = stubHandler{}

func TestResolveStatsHandler(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	resp := new(dns.Msg)
	resp.SetReply(req)

	rec := &resolveRecorderStub{}
	got, err := NewResolveStatsHandler(stubHandler{resp: resp}, rec).Handle(t.Context(), req)
	require.NoError(t, err)
	assert.Same(t, resp, got)
	assert.Equal(t, []recordedResponse{{origin: resolvestats.OriginClient, resp: resp}}, rec.recorded)

	rec = &resolveRecorderStub{}
	_, err = NewResolveStatsHandler(stubHandler{err: errors.New("upstream down")}, rec).Handle(t.Context(), req)
	require.Error(t, err)
	assert.Empty(t, rec.recorded, "a failed lookup has no answers to record")
}
