package auth

import (
	"github.com/go-thx/thx"
	"github.com/go-thx/thx/internal"
)

// GetHandler is a handler function for authenticated GET requests.
// T is the auth entity type, Q is the query parameter type.
type GetHandler[T, Q any] func(Context[T], Q) thx.Result

// Handler is a handler function for authenticated requests with a body.
// T is the auth entity type, Q is the query parameter type, I is the input type.
type Handler[T, Q, I any] func(Context[T], Q, I) thx.Result

// Get wraps an authenticated GET handler into a standard handler function,
// enforcing the given rules before the handler runs. Unlike rules on a guard,
// these run after the route matched, so they can read path parameters.
// Panics if the request is not authorized — use WithGuard to protect routes.
func Get[T, Q any](handler GetHandler[T, Q], rules ...Rule[T]) func(thx.Context, Q) thx.Result {
	rule := All(rules...)

	return func(ctx thx.Context, query Q) thx.Result {
		if !ctx.IsAuthorized() {
			panic("auth: unauthorized request reached auth handler (missing WithGuard?)")
		}

		if result, ok := evaluate(ctx, rule, optionsFrom(ctx)); !ok {
			return result
		}

		return handler(internal.NewAuthContext[T](ctx), query)
	}
}

// Route wraps an authenticated handler (with body) into a standard handler
// function, enforcing the given rules before the handler runs.
// Panics if the request is not authorized — use WithGuard to protect routes.
func Route[T, Q, I any](handler Handler[T, Q, I], rules ...Rule[T]) func(thx.Context, Q, I) thx.Result {
	rule := All(rules...)

	return func(ctx thx.Context, query Q, in I) thx.Result {
		if !ctx.IsAuthorized() {
			panic("auth: unauthorized request reached auth handler (missing WithGuard?)")
		}

		if result, ok := evaluate(ctx, rule, optionsFrom(ctx)); !ok {
			return result
		}

		return handler(internal.NewAuthContext[T](ctx), query, in)
	}
}
