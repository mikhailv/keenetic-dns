package dnssvc

import (
	"context"

	"github.com/miekg/dns"
)

type Handler func(ctx context.Context, req *dns.Msg) (*dns.Msg, error)

type Resolver interface {
	Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error)
}

type ResolverFunc Handler

func (r ResolverFunc) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	return r(ctx, msg)
}

type Middleware func(handler Handler) Handler

var NopMiddleware = func(handler Handler) Handler { return handler }

func NewMiddlewareChainHandler(middlewares []Middleware, handler Handler) Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func EnableMiddleware(middleware Middleware, enable bool) Middleware {
	if enable {
		return middleware
	}
	return NopMiddleware
}

func DisableMiddleware(middleware Middleware, disable bool) Middleware {
	return EnableMiddleware(middleware, !disable)
}
