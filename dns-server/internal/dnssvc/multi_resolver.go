package dnssvc

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"sync/atomic"

	"github.com/miekg/dns"
)

func NewMultiProviderResolver(providers []Provider) Resolver {
	return multiProviderResolver(providers)
}

var _ Resolver = multiProviderResolver{}

type multiProviderResolver []Provider

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
	var badResp *dns.Msg

	for _, key := range priorityKeys {
		for resp, err := range resolveInParallel(ctx, resolvers[key], msg) {
			if isSucceededResponse(resp) {
				return resp, nil
			}
			if err != nil {
				errs = append(errs, err)
			} else {
				badResp = resp
			}
		}
	}

	if badResp != nil {
		return badResp, nil
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

func resolveInParallel(ctx context.Context, resolvers []Resolver, msg *dns.Msg) iter.Seq2[*dns.Msg, error] {
	if len(resolvers) == 0 {
		return noResolversProvided
	}

	return func(yield func(*dns.Msg, error) bool) {
		if len(resolvers) == 1 {
			yield(resolvers[0].Resolve(ctx, msg))
			return
		}

		type JobResult struct {
			msg *dns.Msg
			err error
		}

		var pending atomic.Int32
		pending.Store(int32(len(resolvers)))

		resultQueue := make(chan JobResult, len(resolvers))

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for i := range resolvers {
			go func(resolver Resolver) {
				resp, err := resolver.Resolve(ctx, msg)
				resultQueue <- JobResult{resp, err}
				if pending.Add(-1) == 0 {
					close(resultQueue)
				}
			}(resolvers[i])
		}

		for it := range resultQueue {
			if !yield(it.msg, it.err) {
				return
			}
		}
	}
}

var errNoResolversProvided = errors.New("no resolvers provided")

func noResolversProvided(yield func(*dns.Msg, error) bool) {
	yield(nil, errNoResolversProvided)
}
