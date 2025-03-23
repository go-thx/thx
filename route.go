package thx

import (
	"context"
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

type route[Q, I, O any] struct {
	method     func(mux *http.ServeMux) func(pattern string, handlerFn http.HandlerFunc)
	path       string
	getHandler GetHandler[Q, O]
	handler    Handler[Q, I, O]
}

func (r *route[Q, I, O]) Apply(router *Router) {
	method := r.method(router.Mux)
	path := filepath.Join(router.Path, r.path)

	method(path, func(res http.ResponseWriter, req *http.Request) {
		defer func() {
			if reason := recover(); reason != nil {
				if err, ok := reason.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					// We do not recover http.ErrAbortHandler so the response
					// to the client is aborted, this should not be logged.
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

				ctx := internal.NewContext(req, res)

				if router.ErrorHandler != nil {
					reasonErr, ok := reason.(error)
					if !ok {
						reasonErr = errors.New(fmt.Sprintf("panic: %v", reason))
					}

					router.ErrorHandler(ctx, res, req, reasonErr)
				}
			}
		}()

		// contentType := req.Header.Get("Content-Type")
		accept := req.Header.Get("Accept")

		// map query params

		decoder := schema.NewDecoder()

		var queryData Q

		if err := decoder.Decode(&queryData, req.URL.Query()); err != nil {
			slog.WarnContext(req.Context(), "Failed to decode query params.",
				"error", err,
			)

			res.WriteHeader(http.StatusBadRequest)

			if router.ErrorHandler != nil {
				ctx := internal.NewContext(req, res)
				router.ErrorHandler(ctx, res, req, err)
			}

			return
		}

		validate := validator.New(validator.WithRequiredStructEnabled())

		err := validate.StructCtx(req.Context(), queryData)
		if err != nil {
			slog.WarnContext(req.Context(), "Validation of query params failed.",
				"error", err,
			)

			res.WriteHeader(http.StatusBadRequest)

			if router.ErrorHandler != nil {
				ctx := internal.NewContext(req, res)
				router.ErrorHandler(ctx, res, req, err)
			}

			return
		}

		// jsonParams, err := json.Marshal(queryParams)
		// if err != nil {
		// 	slog.ErrorContext(req.Context(), "Failed to marshal query params.",
		// 		"error", err,
		// 	)
		//
		// 	res.WriteHeader(http.StatusInternalServerError)
		// 	return
		// }

		// if err := json.Unmarshal(jsonParams, &queryData); err != nil {
		//
		// }

		ctx := internal.NewContext(req, res)

		var out O

		if r.getHandler != nil {
			out = r.getHandler(ctx, queryData)
		} else {
			// map input body

			var in I

			if err := req.ParseForm(); err != nil {
				slog.ErrorContext(req.Context(), "Failed to parse form.",
					"error", err,
				)

				res.WriteHeader(http.StatusBadRequest)
				return
			}

			if err := decoder.Decode(&in, req.PostForm); err != nil {
				slog.ErrorContext(req.Context(), "Failed to decode form.",
					"error", err,
				)

				res.WriteHeader(http.StatusBadRequest)
				return
			}

			// if strings.Contains(contentType, "application/json") {
			// 	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
			// 		slog.DebugContext(ctx, "Failed to decode request body.",
			// 			"error", err,
			// 		)
			//
			// 		res.WriteHeader(http.StatusBadRequest)
			// 		return
			// 	}
			// }

			out = r.handler(ctx, queryData, in)
		}

		if router.Layout != nil {
			if comp, ok := any(out).(templ.Component); ok {
				if err := router.Layout(ctx, comp).Render(ctx, res); err != nil {
					slog.ErrorContext(ctx, "Failed to render component.",
						"error", err,
					)

					res.WriteHeader(http.StatusInternalServerError)
					return
				}

				return
			}
		}

		if outer, ok := any(out).(Outer); ok {
			if err := outer.Out(ctx, res); err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				return
			}

			return
		}

		if accept == "application/json" {
			if err := json.NewEncoder(res).Encode(out); err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				return
			}

			return
		}
	})
}

type Layout func(templ.Component) templ.Component
type LayoutWhat func(Context, templ.Component) templ.Component
type Component func(internal.Context) templ.Component

type Wrapper func(*Router)

func (w Wrapper) Apply(router *Router) {
	w(router)
}

func layoutLayout(inner Layout, outer LayoutWhat) LayoutWhat {
	return func(ctx internal.Context, comp templ.Component) templ.Component {
		if ctx.IsWithoutLayouts() {
			return comp
		}

		if inner != nil && outer != nil {
			return outer(ctx, inner(comp))
		}

		if inner != nil {
			return inner(comp)
		}

		if outer != nil {
			return outer(ctx, comp)
		}

		return comp
	}
}

func WithPath(path string, routes []Route) Routes {
	var out Routes

	for _, route := range routes {
		out = append(out, Wrapper(func(r *Router) {
			route.Apply(&Router{
				Mux:             r.Mux,
				Path:            filepath.Join(r.Path, path),
				Layout:          r.Layout,
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
				Layout:          layoutLayout(layout, r.Layout),
				ErrorHandler:    r.ErrorHandler,
				NotFoundHandler: r.NotFoundHandler,
			})
		}))
	}

	return out
}

func WithPathAndLayout(path string, layout Layout, routes []Route) Routes {
	var out Routes

	for _, route := range routes {
		out = append(out, Wrapper(func(r *Router) {
			route.Apply(&Router{
				Mux:             r.Mux,
				Path:            filepath.Join(r.Path, path),
				Layout:          layoutLayout(layout, r.Layout),
				ErrorHandler:    r.ErrorHandler,
				NotFoundHandler: r.NotFoundHandler,
			})
		}))
	}

	return out
}

type GetHandler[Q, O any] func(Context, Q) O
type Handler[Q, I, O any] func(Context, Q, I) O

type Routing interface {
	Routes() []Route
}

// func Apply(r Routing) Route {
// 	return func(mux *chi.Mux) {
// 		for _, route := range r.Routes() {
// 			route(mux)
// 		}
// 	}
// }

type Routes []Route

func (r Routes) Apply(router *Router) {
	for _, route := range r {
		route.Apply(router)
	}
}

type Outer interface {
	Out(context.Context, http.ResponseWriter) error
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

		if err := router.Layout(ctx, n.comp(ctx)).Render(ctx, res); err != nil {
			slog.ErrorContext(req.Context(), "Failed to render component.",
				"error", err,
			)

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
		if err := router.Layout(ctx, i.comp(ctx)).Render(ctx, res); err != nil {
			slog.ErrorContext(ctx, "Failed to render component.",
				"error", err,
			)
		}
	}
}
