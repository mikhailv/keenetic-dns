package handlers

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

func NewRawQueryHandler(handler dnssvc.Handler, stream stream.Stream[types.DNSRawQuery]) dnssvc.Handler {
	return &rawQueryHandler{handler, stream}
}

var _ dnssvc.Handler = rawQueryHandler{}

type rawQueryHandler struct {
	handler dnssvc.Handler
	stream  stream.Stream[types.DNSRawQuery]
}

func (s rawQueryHandler) Handle(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	s.appendQuery(ctx, msg, nil)
	resp, err := s.handler.Handle(ctx, msg)
	s.appendQuery(ctx, resp, err)
	return resp, err
}

func (s rawQueryHandler) appendQuery(ctx context.Context, msg *dns.Msg, err error) {
	q := types.DNSRawQuery{
		Time:       types.TimestampFromTime(time.Now()),
		ClientAddr: ctxutil.GetDNSQueryRemoteAddr(ctx),
		Response:   (msg != nil && msg.Response) || err != nil,
		Error:      err,
	}
	if msg != nil {
		q.Msg = util.UnwrapResult(msg.Pack())
	}
	s.stream.Append(q)
}
