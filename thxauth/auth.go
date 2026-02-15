package thxauth

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-thx/thx"
	"github.com/go-thx/thx/internal"
)

type GetHandler[T, Q, O any] func(Context[T], Q) O
type Handler[T, Q, I, O any] func(Context[T], Q, I) O

func Get[T, Q, O any](handler GetHandler[T, Q, O]) thx.GetHandler[Q, O] {
	return func(ctx internal.Context, query Q) O {
		if !ctx.IsAuthorized() {
			panic("thxauth: unauthorized request reached auth handler (missing WithGuard?)")
		}
		return handler(internal.NewAuthContext[T](ctx), query)
	}
}

func Route[T, Q, I, O any](handler Handler[T, Q, I, O]) thx.Handler[Q, I, O] {
	return func(ctx thx.Context, query Q, in I) O {
		if !ctx.IsAuthorized() {
			panic("thxauth: unauthorized request reached auth handler (missing WithGuard?)")
		}
		return handler(internal.NewAuthContext[T](ctx), query, in)
	}
}

type AuthOption func(*authOptions)

type authOptions struct {
	redirectUnauthorized string
	redirectQueryParam   string
}

func RedirectUnauthorized(to string) AuthOption {
	return func(opts *authOptions) {
		opts.redirectUnauthorized = to
	}
}

func RedirectWithCurrentPath(param string) AuthOption {
	return func(opts *authOptions) {
		opts.redirectQueryParam = param
	}
}

// WithGuard prevents unauthorized access to all given routes
// and scopes them under the given path.
func WithGuard(path string, routes []thx.Route, opts ...AuthOption) thx.Routes {
	authOpts := &authOptions{}

	for _, opt := range opts {
		opt(authOpts)
	}

	middleware := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			if internal.NewContext(req, res).IsAuthorized() {
				h.ServeHTTP(res, req)
				return
			}

			if authOpts.redirectUnauthorized == "" {
				res.WriteHeader(http.StatusUnauthorized)
				return
			}

			redirectURL, err := url.Parse(authOpts.redirectUnauthorized)
			if err != nil {
				slog.ErrorContext(req.Context(), "Failed to parse redirect URL.",
					"error", err,
				)

				res.WriteHeader(http.StatusInternalServerError)
				return
			}

			if authOpts.redirectQueryParam != "" && req.URL.RequestURI() != "/" {
				query := redirectURL.Query()
				query.Add(authOpts.redirectQueryParam, req.URL.RequestURI())
				redirectURL.RawQuery = query.Encode()
			}

			http.Redirect(res, req, redirectURL.String(), http.StatusFound)
		})
	}

	return thx.WithPath(path, thx.WithMiddleware(middleware, routes...))
}
