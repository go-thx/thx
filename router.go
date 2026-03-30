package thx

import (
	"net/http"
)

// Route is the building block for defining HTTP routes.
// Implementations register themselves on a Router via Apply.
type Route interface {
	Apply(*Router)
}

// Router holds the routing state accumulated while applying routes.
// It wraps a standard http.ServeMux and tracks layouts, middleware,
// and error handlers for the current scope.
type Router struct {
	// Mux is the underlying ServeMux. Exposed for direct handler registration.
	Mux *http.ServeMux

	// Path is the current URL prefix for scoped routes.
	Path string
	// Layouts are applied outermost-first to rendered components.
	Layouts []Layout
	// Middleware wraps the final handler for the current scope.
	Middleware func(http.Handler) http.Handler

	// ErrorHandler is called on panics and bad requests if set.
	ErrorHandler func(Context, http.ResponseWriter, *http.Request, error)
	// NotFoundHandler is called when no route matches.
	NotFoundHandler func(http.ResponseWriter, *http.Request)
}

// New creates an http.Handler from the given routes.
// It builds an http.ServeMux, applies all routes, and wraps the
// result with any top-level middleware and not-found handling.
func New(routes ...Route) http.Handler {
	mux := http.NewServeMux()

	router := &Router{
		Mux:             mux,
		NotFoundHandler: http.NotFound,
	}

	for _, route := range routes {
		route.Apply(router)
	}

	var handler http.Handler = wrapMux(mux, func() func(http.ResponseWriter, *http.Request) {
		return router.NotFoundHandler
	})

	if router.Middleware != nil {
		handler = router.Middleware(handler)
	}

	return handler
}
