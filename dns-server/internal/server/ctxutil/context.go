package ctxutil

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type contextKeyDNSQueryID struct{}

var queryCounter atomic.Uint64

func WithNewDNSQueryID(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyDNSQueryID{}, types.NewQueryID(time.Now(), queryCounter.Add(1)))
}

func GetDNSQueryID(ctx context.Context) (types.QueryID, bool) {
	id, ok := ctx.Value(contextKeyDNSQueryID{}).(types.QueryID)
	return id, ok
}

type contextKeyDNSQueryClientIP struct{}

func WithDNSQueryClientIP(ctx context.Context, ip types.IPv4) context.Context {
	return context.WithValue(ctx, contextKeyDNSQueryClientIP{}, ip)
}

func GetDNSQueryClientIP(ctx context.Context) (types.IPv4, bool) {
	ip, ok := ctx.Value(contextKeyDNSQueryClientIP{}).(types.IPv4)
	return ip, ok
}

func WithDNSQueryClientAddrString(ctx context.Context, remoteAddr string) context.Context {
	ctx = WithNewDNSQueryID(ctx)
	if ip, ok := parseClientIP(remoteAddr); ok {
		ctx = WithDNSQueryClientIP(ctx, ip)
	}
	return ctx
}

func parseClientIP(remoteAddr string) (types.IPv4, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := types.ParseIPv4(host)
	if err != nil {
		return types.IPv4{}, false
	}
	return ip, true
}
