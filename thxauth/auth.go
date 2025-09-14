package thxauth

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-thx/thx"
	"github.com/go-thx/thx/internal"
)

//
// -- HANDLER
//

type GetHandler[T, Q, O any] func(Context[T], Q) O
type Handler[T, Q, I, O any] func(Context[T], Q, I) O

func Get[T, Q, O any](handler GetHandler[T, Q, O]) thx.GetHandler[Q, O] {
	return func(ctx internal.Context, query Q) O {
		authCtx := internal.NewAuthContext[T](ctx)

		if !authCtx.IsAuthorized() {
			ctx.SetStatus(http.StatusUnauthorized)

			var out O
			return out // TODO: nil deref?
		}

		return handler(authCtx, query)
	}
}

func Route[T, Q, I, O any](handler Handler[T, Q, I, O]) thx.Handler[Q, I, O] {
	return func(ctx thx.Context, query Q, in I) O {
		authCtx := internal.NewAuthContext[T](ctx)

		if !authCtx.IsAuthorized() {
			ctx.SetStatus(http.StatusUnauthorized)

			var out O
			return out // TODO: nil deref?
		}

		return handler(authCtx, query, in)
	}
}

//
// -- ROUTE
//

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

// Guard prevents unauthorized access to all given routes.
func Guard(routes []thx.Route, opts ...AuthOption) thx.Routes {
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

	return thx.Routes{thx.Wrapper(func(router *thx.Router) {
		guardedMux := http.NewServeMux()

		for _, r := range routes {
			r.Apply(&thx.Router{
				Mux:             guardedMux,
				Path:            router.Path,
				Layout:          router.Layout,
				ErrorHandler:    router.ErrorHandler,
				NotFoundHandler: router.NotFoundHandler,
			})
		}

		router.Mux.Handle(router.Path+"/", middleware(guardedMux))
	})}
}

// guardedRoute wraps a route and applies authentication middleware
//type guardedRoute struct {
//	inner      thx.Route
//	middleware func(http.Handler) http.Handler
//}
//
//func (g *guardedRoute) Apply(router *thx.Router) {
//	// Create a middleware-wrapped mux wrapper
//	wrappedMux := &authServeMux{
//		mux:        router.Mux,
//		middleware: g.middleware,
//	}
//
//	// Apply the inner route with the wrapped mux
//	g.inner.Apply(&thx.Router{
//		Mux:             wrappedMux.mux,
//		Path:            router.Path,
//		Layout:          router.Layout,
//		ErrorHandler:    router.ErrorHandler,
//		NotFoundHandler: router.NotFoundHandler,
//	})
//
//	// Note: The middleware is applied via wrappedMux.HandleFunc which wraps handlers before registration
//}
//
//// authServeMux wraps http.ServeMux to apply middleware to all registered handlers
//type authServeMux struct {
//	mux        *http.ServeMux
//	middleware func(http.Handler) http.Handler
//}
//
//func (a *authServeMux) Handle(pattern string, handler http.Handler) {
//	a.mux.Handle(pattern, a.middleware(handler))
//}
//
//func (a *authServeMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
//	a.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
//		a.middleware(http.HandlerFunc(handler)).ServeHTTP(w, r)
//	})
//}
