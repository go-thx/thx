package auth

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-thx/thx"
	"github.com/go-thx/thx/internal"
)

// GetHandler is a handler function for authenticated GET requests.
// T is the auth entity type, Q is the query parameter type.
type GetHandler[T, Q any] func(Context[T], Q) thx.Result

// Handler is a handler function for authenticated requests with a body.
// T is the auth entity type, Q is the query parameter type, I is the input type.
type Handler[T, Q, I any] func(Context[T], Q, I) thx.Result

// Get wraps an authenticated GET handler into a standard GetHandler.
// Panics if the request is not authorized — use WithGuard to protect routes.
func Get[T, Q any](handler GetHandler[T, Q]) func(thx.Context, Q) thx.Result {
	return func(ctx thx.Context, query Q) thx.Result {
		if !ctx.IsAuthorized() {
			panic("auth: unauthorized request reached auth handler (missing WithGuard?)")
		}
		return handler(internal.NewAuthContext[T](ctx), query)
	}
}

// Route wraps an authenticated handler (with body) into a standard Handler.
// Panics if the request is not authorized — use WithGuard to protect routes.
func Route[T, Q, I any](handler Handler[T, Q, I]) func(thx.Context, Q, I) thx.Result {
	return func(ctx thx.Context, query Q, in I) thx.Result {
		if !ctx.IsAuthorized() {
			panic("auth: unauthorized request reached auth handler (missing WithGuard?)")
		}
		return handler(internal.NewAuthContext[T](ctx), query, in)
	}
}

// GuardOption configures the behavior of WithGuard.
type GuardOption func(*guardOptions)

type guardOptions struct {
	redirectUnauthorized string
	redirectQueryParam   string
}

// RedirectUnauthorized configures WithGuard to redirect unauthenticated
// requests to the given URL instead of returning 401.
func RedirectUnauthorized(to string) GuardOption {
	return func(opts *guardOptions) {
		opts.redirectUnauthorized = to
	}
}

// RedirectWithCurrentPath appends the original request URI as a query
// parameter on the redirect URL, enabling post-login return navigation.
func RedirectWithCurrentPath(param string) GuardOption {
	return func(opts *guardOptions) {
		opts.redirectQueryParam = param
	}
}

// WithGuard prevents unauthorized access to all given routes
// and scopes them under the given path.
func WithGuard(path string, routes []thx.Route, opts ...GuardOption) thx.Routes {
	guardOpts := &guardOptions{}

	for _, opt := range opts {
		opt(guardOpts)
	}

	middleware := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			if internal.NewContext(req, res).IsAuthorized() {
				h.ServeHTTP(res, req)
				return
			}

			if guardOpts.redirectUnauthorized == "" {
				res.WriteHeader(http.StatusUnauthorized)
				return
			}

			redirectURL, err := url.Parse(guardOpts.redirectUnauthorized)
			if err != nil {
				slog.ErrorContext(req.Context(), "Failed to parse redirect URL.",
					"error", err,
				)

				res.WriteHeader(http.StatusInternalServerError)
				return
			}

			if guardOpts.redirectQueryParam != "" && req.URL.RequestURI() != "/" {
				query := redirectURL.Query()
				query.Add(guardOpts.redirectQueryParam, req.URL.RequestURI())
				redirectURL.RawQuery = query.Encode()
			}

			http.Redirect(res, req, redirectURL.String(), http.StatusFound)
		})
	}

	return thx.WithPath(path, thx.WithMiddleware(middleware, routes...))
}
