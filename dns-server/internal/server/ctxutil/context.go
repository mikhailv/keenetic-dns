package ctxutil

import (
	"context"
	"net"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type contextKeyDNSQueryClientIP struct{}

func WithDNSQueryClientIP(ctx context.Context, ip types.IPv4) context.Context {
	return context.WithValue(ctx, contextKeyDNSQueryClientIP{}, ip)
}

func GetDNSQueryClientIP(ctx context.Context) (types.IPv4, bool) {
	ip, ok := ctx.Value(contextKeyDNSQueryClientIP{}).(types.IPv4)
	return ip, ok
}

func WithDNSQueryClientAddrString(ctx context.Context, remoteAddr string) context.Context {
	return WithDNSQueryClientIP(ctx, parseClientIP(remoteAddr))
}

func parseClientIP(remoteAddr string) types.IPv4 {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := types.ParseIPv4(host)
	if err != nil {
		return types.IPv4{}
	}
	return ip
}
