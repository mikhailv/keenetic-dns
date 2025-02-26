package ctxutil

import "context"

type contextKeyDNSQueryRemoteAddr struct{}

func WithDNSQueryRemoteAddr(ctx context.Context, remoteAddr string) context.Context {
	return context.WithValue(ctx, contextKeyDNSQueryRemoteAddr{}, remoteAddr)
}

func GetDNSQueryRemoteAddr(ctx context.Context) string {
	addr, _ := ctx.Value(contextKeyDNSQueryRemoteAddr{}).(string)
	return addr
}
