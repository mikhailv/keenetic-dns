package dnssvc

import (
	"context"

	"github.com/miekg/dns"
)

var _ Handler = HandlerFunc(nil)

type HandlerFunc func(ctx context.Context, req *dns.Msg) (*dns.Msg, error)

func (h HandlerFunc) Handle(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	return h(ctx, msg)
}

type Handler interface {
	Handle(ctx context.Context, msg *dns.Msg) (*dns.Msg, error)
}

type Resolver interface {
	Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error)
	Close() error
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

type middlewareChainResolver struct {
	Resolver
	handler Handler
}

func NewMiddlewareChainResolver(middlewares []Middleware, resolver Resolver) Resolver {
	if len(middlewares) == 0 {
		return resolver
	}
	return middlewareChainResolver{resolver, NewMiddlewareChainHandler(middlewares, HandlerFunc(resolver.Resolve))}
}

func (s middlewareChainResolver) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	return s.handler.Handle(ctx, msg)
}
