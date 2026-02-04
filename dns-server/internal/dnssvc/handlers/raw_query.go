package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
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
	s.appendRawQuery(ctx, false, msg.String())
	resp, err := s.handler.Handle(ctx, msg)
	if err != nil {
		s.appendRawQuery(ctx, true, fmt.Sprintf("ERROR: query (id: %d) failed: %v", msg.Id, err))
	} else {
		s.appendRawQuery(ctx, true, resp.String())
	}
	return resp, err
}

func (s rawQueryHandler) appendRawQuery(ctx context.Context, response bool, text string) {
	s.stream.Append(types.DNSRawQuery{
		Time:       time.Now(),
		ClientAddr: ctxutil.GetDNSQueryRemoteAddr(ctx),
		Response:   response,
		Text:       text,
	})
}
