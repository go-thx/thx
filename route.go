package thx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime/debug"

	"github.com/go-thx/thx/internal"
	"github.com/pkg/errors"

	"github.com/a-h/templ"
	"github.com/go-playground/validator/v10"
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
			reasonErr = errors.New(fmt.Sprintf("panic: %v", reason))
		}

		router.ErrorHandler(ctx, res, req, reasonErr)
	}
}

func decodeQuery[Q any](req *http.Request, res http.ResponseWriter, router *Router) (Q, bool) {
	decoder := schema.NewDecoder()

	var queryData Q

	if err := decoder.Decode(&queryData, req.URL.Query()); err != nil {
		res.WriteHeader(http.StatusBadRequest)

		if router.ErrorHandler != nil {
			ctx := internal.NewContext(req, res)
			router.ErrorHandler(ctx, res, req, err)
		}

		return queryData, false
	}

	validate := validator.New(validator.WithRequiredStructEnabled())

	if err := validate.StructCtx(req.Context(), queryData); err != nil {
		res.WriteHeader(http.StatusBadRequest)

		if router.ErrorHandler != nil {
			ctx := internal.NewContext(req, res)
			router.ErrorHandler(ctx, res, req, err)
		}

		return queryData, false
	}

	return queryData, true
}

type route[Q, I, O any] struct {
	method     string
	path       string
	getHandler GetHandler[Q, O]
	handler    Handler[Q, I, O]
}

func decodeForm[I any](req *http.Request, res http.ResponseWriter, router *Router) (I, bool) {
	var in I

	if err := req.ParseForm(); err != nil {
		res.WriteHeader(http.StatusBadRequest)

		if router.ErrorHandler != nil {
			ctx := internal.NewContext(req, res)
			router.ErrorHandler(ctx, res, req, err)
		}

		return in, false
	}

	decoder := schema.NewDecoder()
	if err := decoder.Decode(&in, req.PostForm); err != nil {
		res.WriteHeader(http.StatusBadRequest)

		if router.ErrorHandler != nil {
			ctx := internal.NewContext(req, res)
			router.ErrorHandler(ctx, res, req, err)
		}

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

func renderOutput[O any](out O, ctx internal.Context, router *Router, res http.ResponseWriter, accept string) {
	if comp, ok := any(out).(templ.Component); ok && len(router.Layouts) > 0 {
		if err := applyLayouts(ctx, comp, router.Layouts).Render(ctx, res); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	if view, ok := any(out).(View); ok {
		if err := view.Out(ctx, res); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	if accept == "application/json" {
		if err := json.NewEncoder(res).Encode(out); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func (r *route[Q, I, O]) Apply(router *Router) {
	path := filepath.Join(router.Path, r.path)

	router.Mux.HandleFunc(r.method+" "+path, func(res http.ResponseWriter, req *http.Request) {
		defer handlePanic(path, router, res, req)

		accept := req.Header.Get("Accept")

		queryData, ok := decodeQuery[Q](req, res, router)
		if !ok {
			return
		}

		ctx := internal.NewContext(req, res)

		htmx := ctx.HTMX()
		if htmx.IsRequest() {
			res.Header().Add("Vary", "HX-Request")

			if !htmx.IsBoosted() {
				ctx.WithoutLayouts()
			}
		}

		if r.getHandler != nil {
			renderOutput(r.getHandler(ctx, queryData), ctx, router, res, accept)
			return
		}

		in, ok := decodeForm[I](req, res, router)
		if !ok {
			return
		}

		renderOutput(r.handler(ctx, queryData, in), ctx, router, res, accept)
	})
}

type Layout func(templ.Component) templ.Component
type Component func(internal.Context) templ.Component

type Wrapper func(*Router)

func (w Wrapper) Apply(router *Router) {
	w(router)
}

func WithPath(path string, routes []Route) Routes {
	var out Routes

	for _, route := range routes {
		out = append(out, Wrapper(func(r *Router) {
			route.Apply(&Router{
				Mux:             r.Mux,
				Path:            filepath.Join(r.Path, path),
				Layouts:         r.Layouts,
				ErrorHandler:    r.ErrorHandler,
				NotFoundHandler: r.NotFoundHandler,
			})
		}))
	}

	return out
}

func WithLayout(layout Layout, routes []Route) Routes {
	var out Routes

	for _, route := range routes {
		out = append(out, Wrapper(func(r *Router) {
			route.Apply(&Router{
				Mux:             r.Mux,
				Path:            r.Path,
				Layouts:         append([]Layout{layout}, r.Layouts...),
				ErrorHandler:    r.ErrorHandler,
				NotFoundHandler: r.NotFoundHandler,
			})
		}))
	}

	return out
}

type GetHandler[Q, O any] func(Context, Q) O
type Handler[Q, I, O any] func(Context, Q, I) O

type Routes []Route

func (r Routes) Apply(router *Router) {
	for _, route := range r {
		route.Apply(router)
	}
}

// 404

func HandleNotFound(comp Component) Route {
	return &notFound{comp: comp}
}

type notFound struct {
	comp Component
}

func (n *notFound) Apply(router *Router) {
	// Note: http.ServeMux doesn't have a built-in NotFound handler like chi
	// Custom NotFound handling would need to be implemented by wrapping the mux
	// For now, we store the handler in the Router for potential use by the application
	router.NotFoundHandler = func(res http.ResponseWriter, req *http.Request) {
		ctx := internal.NewContext(req, res)

		res.WriteHeader(http.StatusNotFound)

		if err := applyLayouts(ctx, n.comp(ctx), router.Layouts).Render(ctx, res); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// 500

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
