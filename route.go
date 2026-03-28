package thx

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-thx/thx/internal"
	"github.com/gorilla/schema"
)

func handlePanic(path string, router *Router, res http.ResponseWriter, req *http.Request) {
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

	if router.ErrorHandler != nil {
		ctx := internal.NewContext(req, res)

		reasonErr, ok := reason.(error)
		if !ok {
			reasonErr = fmt.Errorf("panic: %v", reason)
		}

		router.ErrorHandler(ctx, res, req, reasonErr)
	}
}

var schemaDecoder = schema.NewDecoder()

func handleBadRequest(err error, req *http.Request, res http.ResponseWriter, router *Router) {
	res.WriteHeader(http.StatusBadRequest)

	if router.ErrorHandler != nil {
		ctx := internal.NewContext(req, res)
		router.ErrorHandler(ctx, res, req, err)
	}
}

func decodeQuery[Q any](req *http.Request, res http.ResponseWriter, router *Router) (Q, bool) {
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

	if err := schemaDecoder.Decode(&queryData, query); err != nil {
		handleBadRequest(err, req, res, router)
		return queryData, false
	}

	return queryData, true
}

func routePath(base, path string) string {
	p := filepath.Join(base, path)
	if p == "/" {
		return "/{$}"
	}
	return p
}

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

type route[Q, I any] struct {
	method     string
	path       string
	getHandler GetHandler[Q]
	handler    Handler[Q, I]
}

func decodeForm[I any](req *http.Request, res http.ResponseWriter, router *Router) (I, bool) {
	var in I

	ct := req.Header.Get("Content-Type")
	if ct == "application/json" || strings.HasPrefix(ct, "application/json;") {
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
			handleBadRequest(err, req, res, router)
			return in, false
		}

		return in, true
	}

	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			handleBadRequest(err, req, res, router)
			return in, false
		}
	} else {
		if err := req.ParseForm(); err != nil {
			handleBadRequest(err, req, res, router)
			return in, false
		}
	}

	if err := schemaDecoder.Decode(&in, req.PostForm); err != nil {
		handleBadRequest(err, req, res, router)
		return in, false
	}

	return in, true
}

func applyLayouts(ctx internal.Context, comp templ.Component, layouts []Layout) templ.Component {
	if ctx.IsWithoutLayouts() {
		return comp
	}
	for _, layout := range layouts {
		comp = layout(comp)
	}
	return comp
}

func writeResult(res http.ResponseWriter, result Result) {
	if result == nil {
		return
	}
	if err := result.WriteResult(res); err != nil {
		res.WriteHeader(http.StatusInternalServerError)
	}
}

func (r *route[Q, I]) Apply(router *Router) {
	path := routePath(router.Path, r.path)

	router.Mux.HandleFunc(r.method+" "+path, func(res http.ResponseWriter, req *http.Request) {
		defer handlePanic(path, router, res, req)

		queryData, ok := decodeQuery[Q](req, res, router)
		if !ok {
			return
		}

		ctx := internal.NewContext(req, res)
		ctx.SetValue(layoutsKey{}, router.Layouts)

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

		in, ok := decodeForm[I](req, res, router)
		if !ok {
			return
		}

		writeResult(res, r.handler(ctx, queryData, in))
	})
}

type Layout func(templ.Component) templ.Component
type Component func(internal.Context) templ.Component

type Wrapper func(*Router)

func (w Wrapper) Apply(router *Router) {
	w(router)
}

func use(mw func(http.Handler) http.Handler) Route {
	return Wrapper(func(r *Router) {
		if r.Middleware != nil {
			prev := r.Middleware
			r.Middleware = func(h http.Handler) http.Handler {
				return prev(mw(h))
			}
		} else {
			r.Middleware = mw
		}
	})
}

func WithMiddleware(mw func(http.Handler) http.Handler, routes ...Route) Routes {
	out := make(Routes, 0, len(routes)+1)
	out = append(out, use(mw))
	out = append(out, routes...)
	return out
}

func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			h = mws[i](h)
		}
		return h
	}
}

func WithPath(path string, routes ...Route) Routes {
	return Routes{Wrapper(func(r *Router) {
		subMux := http.NewServeMux()

		subRouter := &Router{
			Mux:          subMux,
			Layouts:      r.Layouts,
			ErrorHandler: r.ErrorHandler,
		}

		for _, route := range routes {
			route.Apply(subRouter)
		}

		prefix := filepath.Join(r.Path, path)

		inner := wrapMux(subMux, func() func(http.ResponseWriter, *http.Request) {
			if subRouter.NotFoundHandler != nil {
				return subRouter.NotFoundHandler
			}
			return r.NotFoundHandler
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
		if subRouter.Middleware != nil {
			subtreeHandler = subRouter.Middleware(subtreeHandler)
			exactHandler = subRouter.Middleware(exactHandler)
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

func WithLayout(layout Layout, routes ...Route) Routes {
	return Routes{Wrapper(func(r *Router) {
		inner := &Router{
			Mux:          r.Mux,
			Path:         r.Path,
			Layouts:      append([]Layout{layout}, r.Layouts...),
			ErrorHandler: r.ErrorHandler,
			Middleware:   r.Middleware,
		}

		for _, route := range routes {
			route.Apply(inner)
		}

		r.ErrorHandler = inner.ErrorHandler
		r.NotFoundHandler = inner.NotFoundHandler
		r.Middleware = inner.Middleware
	})}
}

type GetHandler[Q any] func(Context, Q) Result
type Handler[Q, I any] func(Context, Q, I) Result

type Routes []Route

func (r Routes) Apply(router *Router) {
	for _, route := range r {
		route.Apply(router)
	}
}

func HandleNotFound(comp Component) Route {
	return &notFound{comp: comp}
}

type notFound struct {
	comp Component
}

func (n *notFound) Apply(router *Router) {
	router.NotFoundHandler = func(res http.ResponseWriter, req *http.Request) {
		ctx := internal.NewContext(req, res)

		res.WriteHeader(http.StatusNotFound)

		if err := applyLayouts(ctx, n.comp(ctx), router.Layouts).Render(ctx, res); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func HandleInternalError(comp Component) Route {
	return &internalError{comp: comp}
}

type internalError struct {
	comp Component
}

func (i *internalError) Apply(router *Router) {
	router.ErrorHandler = func(ctx Context, res http.ResponseWriter, req *http.Request, err error) {
		if err := applyLayouts(ctx, i.comp(ctx), router.Layouts).Render(ctx, res); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
		}
	}
}
