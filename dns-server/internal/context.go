package internal

import "context"

type contextKeyDNSQueryRemoteAddr struct{}

func withDNSQueryRemoteAddr(ctx context.Context, remoteAddr string) context.Context {
	return context.WithValue(ctx, contextKeyDNSQueryRemoteAddr{}, remoteAddr)
}

func getDNSQueryRemoteAddr(ctx context.Context) string {
	addr, _ := ctx.Value(contextKeyDNSQueryRemoteAddr{}).(string)
	return addr
}
