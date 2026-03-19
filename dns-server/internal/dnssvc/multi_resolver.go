package dnssvc

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

var errNoResolversProvided = errors.New("no resolvers provided")

func NewMultiProviderResolver(providers []Provider) Resolver {
	return multiProviderResolver(providers)
}

var _ Resolver = multiProviderResolver{}

type multiProviderResolver []Provider

func (s multiProviderResolver) Name() string {
	return "multi_resolver"
}

func (s multiProviderResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resolvers := map[int32][]Resolver{}
	for _, p := range s {
		if res := p.MatchQuery(msg); res.Score >= 0 {
			resolvers[res.Score] = append(resolvers[res.Score], p)
		}
	}

	if len(resolvers) == 0 {
		return RefusedResponse(msg), fmt.Errorf("unable to choose DNS provider to process query: %+v", *msg)
	}

	priorityKeys := slices.AppendSeq(make([]int32, 0, len(resolvers)), maps.Keys(resolvers))
	slices.Sort(priorityKeys)
	slices.Reverse(priorityKeys) // in descending order

	var errs []error
	var badResult *resolveJobResult

	for _, key := range priorityKeys {
		for r := range resolveInParallel(ctx, resolvers[key], msg) {
			if r.err != nil {
				errs = append(errs, r.err)
				continue
			}
			if isSucceededResponse(r.resp) {
				SetResolvedByInContext(ctx, r.resolver.Name(), r.duration)
				return r.resp, nil
			}
			badResult = &r
		}
	}

	if badResult != nil {
		SetResolvedByInContext(ctx, badResult.resolver.Name(), badResult.duration)
		return badResult.resp, nil
	}
	return RefusedResponse(msg), errors.Join(errs...)
}

func (s multiProviderResolver) Close() error {
	errs := make([]error, len(s))
	for i, p := range s {
		errs[i] = p.Close()
	}
	return errors.Join(errs...)
}

type resolveJobResult struct {
	resolver Resolver
	resp     *dns.Msg
	err      error
	duration time.Duration
}

func resolveInParallel(ctx context.Context, resolvers []Resolver, msg *dns.Msg) iter.Seq[resolveJobResult] {
	if len(resolvers) == 0 {
		return func(yield func(resolveJobResult) bool) {
			yield(resolveJobResult{nil, nil, errNoResolversProvided, 0})
		}
	}

	resolve := func(ctx context.Context, resolver Resolver) resolveJobResult {
		st := time.Now()
		resp, err := resolver.Resolve(ctx, msg)
		return resolveJobResult{resolver, resp, err, time.Since(st)}
	}

	return func(yield func(resolveJobResult) bool) {
		if len(resolvers) == 1 {
			yield(resolve(ctx, resolvers[0]))
			return
		}

		var pending atomic.Int32
		pending.Store(int32(len(resolvers)))

		resultQueue := make(chan resolveJobResult, len(resolvers))

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, resolver := range resolvers {
			go func() {
				resultQueue <- resolve(ctx, resolver)
				if pending.Add(-1) == 0 {
					close(resultQueue)
				}
			}()
		}

		for res := range resultQueue {
			if !yield(res) {
				return
			}
		}
	}
}

type contextKeyResolvedBy struct{}

// WithResolvedByContext returns a context that can track which resolver handled a query.
// Use SetResolvedByInContext to set the resolver name.
func WithResolvedByContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyResolvedBy{}, &types.ResolvedBy{})
}

// SetResolvedByInContext sets the resolver name in the context.
// The context must be initialized with WithResolvedByContext first.
func SetResolvedByInContext(ctx context.Context, resolver string, duration time.Duration) {
	if v, ok := ctx.Value(contextKeyResolvedBy{}).(*types.ResolvedBy); ok {
		*v = types.ResolvedBy{Resolver: resolver, Duration: duration.Seconds()}
	}
}

func GetResolvedBy(ctx context.Context) types.ResolvedBy {
	if v, ok := ctx.Value(contextKeyResolvedBy{}).(*types.ResolvedBy); ok {
		return *v
	}
	return types.ResolvedBy{}
}
