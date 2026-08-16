package handlers

import (
	"context"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/stream"
)

func NewQueryLogHandler(handler dnssvc.Handler, stream stream.Stream[types.DNSQuery]) dnssvc.Handler {
	return queryLogHandler{handler: handler, stream: stream}
}

var _ dnssvc.Handler = queryLogHandler{}

type queryLogHandler struct {
	handler dnssvc.Handler
	stream  stream.Stream[types.DNSQuery]
}

func (s queryLogHandler) Handle(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	start := time.Now()
	ctx = dnssvc.WithQueryInfo(ctx)

	resp, err := s.handler.Handle(ctx, msg)

	info, ok := dnssvc.GetQueryInfo(ctx)
	if !ok {
		return resp, err
	}

	id, _ := ctxutil.GetDNSQueryID(ctx)
	clientIP, _ := ctxutil.GetDNSQueryClientIP(ctx)
	s.stream.Append(types.DNSQuery{
		ID:           id,
		ClientIP:     clientIP,
		DomainLookup: *info.Lookup,
		Duration:     time.Since(start).Seconds(),
		IPRoutings:   info.IPRoutings,
		ReusedFrom:   info.ReusedFrom,
	})
	return resp, err
}
