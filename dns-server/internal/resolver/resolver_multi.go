package resolver

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

func NewMultiProviderDNSResolver(providers []*DNSProvider) DNSResolver {
	return multiProviderDNSResolver(providers)
}

var _ DNSResolver = multiProviderDNSResolver{}

type multiProviderDNSResolver []*DNSProvider

func (s multiProviderDNSResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resolvers := map[int32][]DNSResolver{}
	for _, provider := range s {
		if score := provider.MatchQuery(msg); score >= 0 {
			resolvers[score] = append(resolvers[score], provider)
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

func resolveInParallel(ctx context.Context, resolvers []DNSResolver, msg *dns.Msg) iter.Seq2[*dns.Msg, error] {
	if len(resolvers) == 0 {
		panic("resolveInParallel: 'resolvers' is empty")
	}

	return func(yield func(*dns.Msg, error) bool) {
		if len(resolvers) == 1 {
			resp, err := resolvers[0].Resolve(ctx, msg)
			yield(resp, err)
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
			go func(resolver DNSResolver) {
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
			if isSucceededResponse(it.msg) {
				// early cancel other resolve requests if any
				return
			}
		}
	}
}
