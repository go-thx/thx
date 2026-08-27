package thx

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	urlpath "path"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-thx/thx/internal"
)

// handlePanic recovers from panics during request handling, logs the
// error with the stack trace, and invokes the router's error handler if set.
// It re-panics on http.ErrAbortHandler to preserve hijack semantics.
func handlePanic(ctx Context, path string, router *Router, res http.ResponseWriter, req *http.Request) {
	reason := recover()
	if reason == nil {
		return
	}

	if err, ok := reason.(error); ok && errors.Is(err, http.ErrAbortHandler) {
		panic(reason)
	}

	slog.ErrorContext(req.Context(), "Recovered from panic.",
		"path", path,
		"reason", reason,
		"stack", string(debug.Stack()),
	)

	if req.Header.Get("Connection") == "Upgrade" {
		return
	}

	res.WriteHeader(http.StatusInternalServerError)

	if router.errorHandler != nil {
		reasonErr, ok := reason.(error)
		if !ok {
			reasonErr = fmt.Errorf("panic: %v", reason)
		}

		router.errorHandler(ctx, res, req, reasonErr)
	}
}

var (
	queryDecoder = newQueryDecoder()
	formDecoder  = newDecoder()
)

// newQueryDecoder creates a decoder configured to ignore unknown query keys.
func newQueryDecoder() *decoder {
	d := newDecoder()
	d.ignoreUnknownKeys(true)
	return d
}

// handleBadRequest reports a failed request decode and delegates to the
// router's error handler. A malformed request is the client's fault (400), an
// undecodable target struct is the handler's (500).
func handleBadRequest(ctx Context, err error, req *http.Request, res http.ResponseWriter, router *Router) {
	status := http.StatusBadRequest
	if errors.Is(err, ErrInvalidTarget) {
		status = http.StatusInternalServerError
	}

	var decErr *DecodeError
	if errors.As(err, &decErr) {
		slog.WarnContext(req.Context(), "Failed to decode request.",
			"path", req.URL.Path,
			"kind", string(decErr.Kind),
			"key", decErr.Key,
			"status", status,
			"error", err,
		)
	}

	res.WriteHeader(status)

	if router.errorHandler != nil {
		router.errorHandler(ctx, res, req, err)
	}
}

// decodeQuery decodes URL query parameters into the typed struct Q.
// For HTMX requests, it also merges query params from HX-Current-URL as a fallback.
func decodeQuery[Q any](ctx Context, req *http.Request, res http.ResponseWriter, router *Router) (Q, bool) {
	var queryData Q

	query := req.URL.Query()

	// For HTMX requests, merge query params from HX-Current-URL as fallback.
	// The XHR URL may differ from the browser's address bar.
	if req.Header.Get("HX-Request") == "true" {
		if hxURL := req.Header.Get("HX-Current-URL"); hxURL != "" {
			if parsed, err := url.Parse(hxURL); err == nil {
				for key, values := range parsed.Query() {
					if _, exists := query[key]; !exists {
						query[key] = values
					}
				}
			}
		}
	}

	if err := queryDecoder.decode(&queryData, query); err != nil {
		handleBadRequest(ctx, err, req, res, router)
		return queryData, false
	}

	return queryData, true
}

// routePath joins the base prefix and the route path, normalizing
// the root path "/" to "/{$}" for exact matching on the ServeMux.
func routePath(base, path string) string {
	p := urlpath.Join(base, path)
	if p == "/" {
		return "/{$}"
	}
	return p
}

// wrapMux returns a handler that delegates to the mux but calls notFound
// when no pattern matches, instead of the mux's default 404.
func wrapMux(mux *http.ServeMux, notFound func() func(http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		_, pattern := mux.Handler(req)
		if pattern == "" {
			notFound()(res, req)
			return
		}
		mux.ServeHTTP(res, req)
	})
}

// route is the core route implementation for all HTTP methods.
// Q is the query parameter type, I is the request body input type.
type route[Q, I any] struct {
	method     string
	path       string
	getHandler getHandler[Q]
	handler    handler[Q, I]
}

// decodeForm decodes the request body into the typed struct I.
// Supports JSON, multipart form data, and URL-encoded form bodies.
func decodeForm[I any](ctx Context, req *http.Request, res http.ResponseWriter, router *Router) (I, bool) {
	var in I

	ct := req.Header.Get("Content-Type")
	if ct == "application/json" || strings.HasPrefix(ct, "application/json;") {
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
			handleBadRequest(ctx, err, req, res, router)
			return in, false
		}

		return in, true
	}

	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			handleBadRequest(ctx, err, req, res, router)
			return in, false
		}
	} else {
		if err := req.ParseForm(); err != nil {
			handleBadRequest(ctx, err, req, res, router)
			return in, false
		}
	}

	if err := formDecoder.decode(&in, req.PostForm); err != nil {
		handleBadRequest(ctx, err, req, res, router)
		return in, false
	}

	return in, true
}

// applyLayouts wraps the component with all layouts unless the context
// opted out via WithoutLayouts — except on HTMX history restore, where
// the full page is always rendered to avoid restoring a bare fragment.
func applyLayouts(ctx internal.Context, comp templ.Component, layouts []Layout) templ.Component {
	hx := ctx.HTMX()
	if ctx.IsWithoutLayouts() && !(hx.IsRequest() && hx.IsHistoryRestoreRequest()) {
		return comp
	}
	for _, layout := range layouts {
		comp = layout(comp)
	}
	return comp
}

// writeResult writes the Result to the response, returning 500 on error.
func writeResult(res http.ResponseWriter, result Result) {
	if result == nil {
		return
	}
	if err := result.WriteResult(res); err != nil {
		res.WriteHeader(http.StatusInternalServerError)
	}
}

// Apply registers the route on the router's mux with query/form decoding,
// HTMX partial rendering, layout application, and panic recovery.
func (r *route[Q, I]) Apply(router *Router) {
	path := routePath(router.path, r.path)

	queryDecoder.warm(reflect.TypeFor[Q]())
	if r.getHandler == nil {
		formDecoder.warm(reflect.TypeFor[I]())
	}

	router.Mux.HandleFunc(r.method+" "+path, func(res http.ResponseWriter, req *http.Request) {
		// One context per request: the decoders, the handler, and both error
		// paths share it, so values set along the way (flash consumption, auth)
		// stay visible to the error page.
		ctx := internal.NewContext(req, res)
		ctx.SetValue(layoutsKey{}, router.layouts)
		if router.flashOOB != nil {
			ctx.SetValue(flashOOBKey{}, router.flashOOB)
		}

		defer handlePanic(ctx, path, router, res, req)
		defer func() {
			if req.MultipartForm != nil {
				if err := req.MultipartForm.RemoveAll(); err != nil {
					slog.ErrorContext(req.Context(), "Failed to remove multipart temp files.",
						"path", path,
						"error", err,
					)
				}
			}
		}()

		queryData, ok := decodeQuery[Q](ctx, req, res, router)
		if !ok {
			return
		}

		htmx := ctx.HTMX()
		if htmx.IsRequest() {
			res.Header().Add("Vary", "HX-Request, HX-Request-Type")

			if htmx.IsPartial() {
				ctx.WithoutLayouts()
			}
		}

		if r.getHandler != nil {
			writeResult(res, r.getHandler(ctx, queryData))
			return
		}

		in, ok := decodeForm[I](ctx, req, res, router)
		if !ok {
			return
		}

		writeResult(res, r.handler(ctx, queryData, in))
	})
}

// Layout wraps a templ component with surrounding markup (e.g. a page shell).
// Layouts nest innermost-first: the first layout in the slice wraps the
// component directly, and each subsequent layout wraps the previous result.
type Layout func(templ.Component) templ.Component

type wrapper func(*Router)

func (w wrapper) Apply(router *Router) {
	w(router)
}

// use returns a Route that registers middleware on the router.
// Multiple middleware are chained in registration order.
func use(mw func(http.Handler) http.Handler) Route {
	return wrapper(func(r *Router) {
		if r.middleware != nil {
			prev := r.middleware
			r.middleware = func(h http.Handler) http.Handler {
				return prev(mw(h))
			}
		} else {
			r.middleware = mw
		}
	})
}

// WithMiddleware wraps the given routes with an HTTP middleware.
// The middleware sees the original request before any route-level processing.
func WithMiddleware(mw func(http.Handler) http.Handler, routes ...Route) Routes {
	out := make(Routes, 0, len(routes)+1)
	out = append(out, use(mw))
	out = append(out, routes...)
	return out
}

// Chain composes multiple middleware into a single middleware.
// Middleware are applied in the order given: the first middleware
// in the list is the outermost wrapper.
func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			h = mws[i](h)
		}
		return h
	}
}

// WithPath groups routes under a URL path prefix. A sub-mux is created
// so that the grouped routes share middleware and not-found handling.
// Trailing slashes are redirected to the canonical path without a slash.
func WithPath(path string, routes ...Route) Routes {
	return Routes{wrapper(func(r *Router) {
		subMux := http.NewServeMux()

		subRouter := &Router{
			Mux:          subMux,
			layouts:      r.layouts,
			flashOOB:     r.flashOOB,
			errorHandler: r.errorHandler,
		}

		for _, route := range routes {
			route.Apply(subRouter)
		}

		prefix := urlpath.Join(r.path, path)

		inner := wrapMux(subMux, func() func(http.ResponseWriter, *http.Request) {
			if subRouter.notFoundHandler != nil {
				return subRouter.notFoundHandler
			}
			return r.notFoundHandler
		})

		// Subtree: strip prefix then route via sub-mux
		var subtreeHandler http.Handler = http.StripPrefix(prefix, inner)

		// Exact prefix: rewrite to "/" then route via sub-mux
		var exactHandler http.Handler = http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			req2 := req.Clone(req.Context())
			req2.URL = &url.URL{}
			*req2.URL = *req.URL
			req2.URL.Path = "/"
			inner.ServeHTTP(res, req2)
		})

		// Middleware wraps outside StripPrefix — sees original request paths
		if subRouter.middleware != nil {
			subtreeHandler = subRouter.middleware(subtreeHandler)
			exactHandler = subRouter.middleware(exactHandler)
		}

		// Subtree: redirect trailing slash, then delegate
		r.Mux.Handle(prefix+"/", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			if strings.HasSuffix(req.URL.Path, "/") {
				target := strings.TrimSuffix(req.URL.Path, "/")
				if req.URL.RawQuery != "" {
					target += "?" + req.URL.RawQuery
				}
				http.Redirect(res, req, target, http.StatusMovedPermanently)
				return
			}
			subtreeHandler.ServeHTTP(res, req)
		}))

		// Exact prefix
		r.Mux.Handle(prefix, exactHandler)
	})}
}

// WithLayout wraps all given routes with an additional layout.
// The new layout becomes the innermost wrapper, closest to the
// rendered component. Layouts from parent scopes remain outermost.
func WithLayout(layout Layout, routes ...Route) Routes {
	return Routes{wrapper(func(r *Router) {
		inner := &Router{
			Mux:             r.Mux,
			path:            r.path,
			layouts:         append([]Layout{layout}, r.layouts...),
			flashOOB:        r.flashOOB,
			errorHandler:    r.errorHandler,
			notFoundHandler: r.notFoundHandler,
			middleware:       r.middleware,
		}

		for _, route := range routes {
			route.Apply(inner)
		}

		r.errorHandler = inner.errorHandler
		if inner.notFoundHandler != nil {
			r.notFoundHandler = inner.notFoundHandler
		}
		r.middleware = inner.middleware
	})}
}

type getHandler[Q any] func(Context, Q) Result

type handler[Q, I any] func(Context, Q, I) Result

// Routes is an ordered collection of Route values that applies as a group.
type Routes []Route

// Apply registers all routes in the collection on the given router.
func (r Routes) Apply(router *Router) {
	for _, route := range r {
		route.Apply(router)
	}
}

// HandleNotFound registers a custom 404 page rendered with the given component.
// The component receives the request context so it can access path values,
// HTMX state, etc.
func HandleNotFound(comp func(Context) templ.Component) Route {
	return &notFound{comp: comp}
}

type notFound struct {
	comp func(Context) templ.Component
}

// Apply sets the router's not-found handler to render the component with layouts.
func (n *notFound) Apply(router *Router) {
	router.notFoundHandler = func(res http.ResponseWriter, req *http.Request) {
		ctx := internal.NewContext(req, res)

		res.WriteHeader(http.StatusNotFound)

		if err := applyLayouts(ctx, n.comp(ctx), router.layouts).Render(ctx, res); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// HandleInternalError registers a custom error page rendered when a handler
// panics or returns an error. The component receives the request context.
func HandleInternalError(comp func(Context) templ.Component) Route {
	return &internalError{comp: comp}
}

type internalError struct {
	comp func(Context) templ.Component
}

// Apply sets the router's error handler to render the component with layouts.
func (i *internalError) Apply(router *Router) {
	router.errorHandler = func(ctx Context, res http.ResponseWriter, req *http.Request, err error) {
		if err := applyLayouts(ctx, i.comp(ctx), router.layouts).Render(ctx, res); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
		}
	}
}
