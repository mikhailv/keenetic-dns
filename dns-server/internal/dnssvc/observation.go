package dnssvc

import (
	"context"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type QueryObservation struct {
	Lookup     *types.DomainLookup
	IPRoutings types.IPRoutings
}

type contextKeyQueryObservation struct{}

func WithQueryObservationContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyQueryObservation{}, &QueryObservation{})
}

func SetQueryObservation(ctx context.Context, obs QueryObservation) {
	if v, ok := ctx.Value(contextKeyQueryObservation{}).(*QueryObservation); ok {
		*v = obs
	}
}

func GetQueryObservation(ctx context.Context) (QueryObservation, bool) {
	v, ok := ctx.Value(contextKeyQueryObservation{}).(*QueryObservation)
	if !ok || v.Lookup == nil {
		return QueryObservation{}, false
	}
	return *v, true
}
