package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-thx/thx"
	"github.com/go-thx/thx/internal"
)

// GuardOption configures how a guard responds to denied requests.
type GuardOption func(*guardOptions)

type guardOptions struct {
	redirectUnauthorized string
	redirectQueryParam   string
	redirectForbidden    string
	onUnauthorized       func(thx.Context, error) thx.Result
	onForbidden          func(thx.Context, error) thx.Result
	onError              func(thx.Context, error) thx.Result
}

// RedirectUnauthorized redirects unauthenticated requests to the given URL
// instead of returning 401. HTMX requests receive an HX-Redirect header so the
// login page replaces the whole document instead of being swapped into a
// fragment.
func RedirectUnauthorized(to string) GuardOption {
	return func(opts *guardOptions) {
		opts.redirectUnauthorized = to
	}
}

// RedirectWithCurrentPath appends the original request URI as a query
// parameter on the unauthorized redirect URL, enabling post-login return
// navigation.
func RedirectWithCurrentPath(param string) GuardOption {
	return func(opts *guardOptions) {
		opts.redirectQueryParam = param
	}
}

// RedirectForbidden redirects requests denied by a Rule to the given URL
// instead of returning 403.
func RedirectForbidden(to string) GuardOption {
	return func(opts *guardOptions) {
		opts.redirectForbidden = to
	}
}

// OnUnauthorized replaces the response for unauthenticated requests.
// The handler receives ErrUnauthenticated.
func OnUnauthorized(handler func(thx.Context, error) thx.Result) GuardOption {
	return func(opts *guardOptions) {
		opts.onUnauthorized = handler
	}
}

// OnForbidden replaces the response for requests denied by a Rule.
// The handler receives the denial error; read its reason with auth.Reason.
func OnForbidden(handler func(thx.Context, error) thx.Result) GuardOption {
	return func(opts *guardOptions) {
		opts.onForbidden = handler
	}
}

// OnError replaces the response for rules that failed to reach a decision
// (for example a permission lookup that could not query the database).
// The default response is 500 with no body.
func OnError(handler func(thx.Context, error) thx.Result) GuardOption {
	return func(opts *guardOptions) {
		opts.onError = handler
	}
}

// WithGuard scopes the given routes under a path and denies access to
// requests that are unauthenticated or rejected by the rule.
//
// The guard runs as middleware, before the request is matched against a route,
// so it also covers not-found responses and any handler registered directly on
// the sub-router's mux. The flip side is that path parameters are not yet
// parsed: a rule evaluated here sees an empty ctx.Param. Rules that depend on
// path parameters belong on the route itself, via Get or Route.
//
// Pass Authenticated[T]() as the rule to require a login without further
// authorization.
func WithGuard[T any](path string, routes []thx.Route, rule Rule[T], opts ...GuardOption) thx.Routes {
	guardOpts := newGuardOptions(opts)

	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			ctx := internal.NewContext(req, res)

			if result, ok := evaluate(ctx, rule, guardOpts); !ok {
				writeResult(req, res, result)
				return
			}

			// Carry the denial handlers to route-level rules, so a rule that
			// runs inside the handler denies the same way the guard does.
			next.ServeHTTP(res, req.WithContext(
				context.WithValue(req.Context(), optionsKey{}, guardOpts),
			))
		})
	}

	return thx.WithPath(path, thx.WithMiddleware(middleware, routes...))
}

func newGuardOptions(opts []GuardOption) *guardOptions {
	guardOpts := &guardOptions{}
	for _, opt := range opts {
		opt(guardOpts)
	}
	return guardOpts
}

type optionsKey struct{}

// optionsFrom returns the denial handlers of the enclosing guard, or the
// defaults when a rule runs on a route outside any guard.
func optionsFrom(ctx thx.Context) *guardOptions {
	if opts, ok := ctx.Value(optionsKey{}).(*guardOptions); ok {
		return opts
	}
	return &guardOptions{}
}

// evaluate applies the rule to the request. It reports whether the request may
// proceed; if not, the returned Result holds the denial response.
func evaluate[T any](ctx thx.Context, rule Rule[T], opts *guardOptions) (thx.Result, bool) {
	if !ctx.IsAuthorized() {
		return opts.denyUnauthorized(ctx), false
	}

	if rule == nil {
		return nil, true
	}

	err := rule(ctx, Subject[T](ctx))
	switch {
	case err == nil:
		return nil, true
	case errors.Is(err, ErrForbidden):
		return opts.denyForbidden(ctx, err), false
	default:
		return opts.failed(ctx, err), false
	}
}

// denyUnauthorized builds the response for a request without an authenticated
// subject.
func (o *guardOptions) denyUnauthorized(ctx thx.Context) thx.Result {
	if o.onUnauthorized != nil {
		return o.onUnauthorized(ctx, ErrUnauthenticated)
	}

	if o.redirectUnauthorized == "" {
		return thx.Status(http.StatusUnauthorized).Empty()
	}

	target, err := url.Parse(o.redirectUnauthorized)
	if err != nil {
		return o.failed(ctx, err)
	}

	if o.redirectQueryParam != "" && ctx.URI() != "/" {
		query := target.Query()
		query.Set(o.redirectQueryParam, ctx.URI())
		target.RawQuery = query.Encode()
	}

	return ctx.Redirect(target.String())
}

// denyForbidden builds the response for a request rejected by a rule.
func (o *guardOptions) denyForbidden(ctx thx.Context, err error) thx.Result {
	if o.onForbidden != nil {
		return o.onForbidden(ctx, err)
	}

	if o.redirectForbidden == "" {
		return thx.Status(http.StatusForbidden).Empty()
	}

	return ctx.Redirect(o.redirectForbidden)
}

// failed builds the response for a rule that could not reach a decision.
func (o *guardOptions) failed(ctx thx.Context, err error) thx.Result {
	slog.ErrorContext(ctx, "Auth rule failed to reach a decision.",
		"uri", ctx.URI(),
		"error", err,
	)

	if o.onError != nil {
		return o.onError(ctx, err)
	}

	return thx.Status(http.StatusInternalServerError).Empty()
}

// writeResult writes a denial response from middleware, where no route
// handler exists to return the Result to.
func writeResult(req *http.Request, res http.ResponseWriter, result thx.Result) {
	if result == nil {
		return
	}

	if err := result.WriteResult(res); err != nil {
		slog.ErrorContext(req.Context(), "Failed to write guard response.",
			"uri", req.RequestURI,
			"error", err,
		)
	}
}
