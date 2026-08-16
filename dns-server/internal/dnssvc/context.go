package dnssvc

import (
	"context"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

//

type contextKeyResolverInfo struct{}

func WithResolverInfo(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyResolverInfo{}, &types.ResolverInfo{})
}

func SetResolverInfo(ctx context.Context, resolver string, duration time.Duration) {
	if v, ok := ctx.Value(contextKeyResolverInfo{}).(*types.ResolverInfo); ok {
		*v = types.ResolverInfo{Name: resolver, Duration: duration.Seconds()}
	}
}

func GetResolverInfo(ctx context.Context) types.ResolverInfo {
	if v, ok := ctx.Value(contextKeyResolverInfo{}).(*types.ResolverInfo); ok {
		return *v
	}
	return types.ResolverInfo{}
}

type QueryInfo struct {
	Lookup     *types.DomainLookup
	IPRoutings types.IPRoutings
}

type contextKeyQueryInfo struct{}

func WithQueryInfo(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyQueryInfo{}, &QueryInfo{})
}

func SetQueryInfo(ctx context.Context, info QueryInfo) {
	if v, ok := ctx.Value(contextKeyQueryInfo{}).(*QueryInfo); ok {
		*v = info
	}
}

func GetQueryInfo(ctx context.Context) (QueryInfo, bool) {
	v, ok := ctx.Value(contextKeyQueryInfo{}).(*QueryInfo)
	if !ok || v.Lookup == nil {
		return QueryInfo{}, false
	}
	return *v, true
}
