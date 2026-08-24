package dnssvc

import (
	"context"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

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

type QueryStatus struct {
	Blocked bool
	Routed  bool
}

type contextKeyQueryStatus struct{}

func WithQueryStatus(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyQueryStatus{}, &QueryStatus{})
}

func SetQueryBlocked(ctx context.Context) {
	if v, ok := ctx.Value(contextKeyQueryStatus{}).(*QueryStatus); ok {
		v.Blocked = true
	}
}

func SetQueryRouted(ctx context.Context) {
	if v, ok := ctx.Value(contextKeyQueryStatus{}).(*QueryStatus); ok {
		v.Routed = true
	}
}

func GetQueryStatus(ctx context.Context) QueryStatus {
	if v, ok := ctx.Value(contextKeyQueryStatus{}).(*QueryStatus); ok {
		return *v
	}
	return QueryStatus{}
}

type QueryInfo struct {
	Lookup     *types.DomainLookup
	IPRoutings types.IPRoutings
	ReusedFrom types.QueryID
	Blocked    *types.BlockInfo
}

func (i QueryInfo) Empty() bool {
	return i.Lookup == nil && i.Blocked == nil
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

func SetQueryBlockInfo(ctx context.Context, info types.BlockInfo) {
	if v, ok := ctx.Value(contextKeyQueryInfo{}).(*QueryInfo); ok {
		v.Blocked = &info
	}
}

func GetQueryInfo(ctx context.Context) QueryInfo {
	if v, ok := ctx.Value(contextKeyQueryInfo{}).(*QueryInfo); ok {
		return *v
	}
	return QueryInfo{}
}
