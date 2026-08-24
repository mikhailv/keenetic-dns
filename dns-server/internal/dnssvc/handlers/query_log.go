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

	info := dnssvc.GetQueryInfo(ctx)
	if info.Empty() {
		return resp, err
	}

	var domain, qtype string
	if dnssvc.HasSingleQuestion(msg) {
		domain = msg.Question[0].Name
		qtype = dns.TypeToString[msg.Question[0].Qtype]
	}

	s.stream.Append(types.DNSQuery{
		ID:         ctxutil.GetDNSQueryID(ctx),
		Time:       types.TimestampFromTime(start),
		ClientIP:   ctxutil.GetDNSQueryClientIP(ctx),
		Domain:     domain,
		QType:      qtype,
		Duration:   time.Since(start).Seconds(),
		ReusedFrom: info.ReusedFrom,
		Blocked:    info.Blocked,
		Lookup:     info.Lookup,
		IPRoutings: info.IPRoutings,
	})
	return resp, err
}
