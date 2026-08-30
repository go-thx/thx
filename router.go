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

	path            string
	layouts         []Layout
	flashOOB        *flashOOBConfig
	middleware      func(http.Handler) http.Handler
	errorHandler    func(Context, http.ResponseWriter, *http.Request, error)
	notFoundHandler func(http.ResponseWriter, *http.Request)
}

// New creates an http.Handler from the given routes.
// It builds an http.ServeMux, applies all routes, and wraps the
// result with any top-level middleware and not-found handling.
func New(routes ...Route) http.Handler {
	mux := http.NewServeMux()

	router := &Router{
		Mux:             mux,
		notFoundHandler: http.NotFound,
	}

	for _, route := range routes {
		route.Apply(router)
	}

	handler := wrapMux(mux, func() func(http.ResponseWriter, *http.Request) {
		return router.notFoundHandler
	})

	if router.middleware != nil {
		handler = router.middleware(handler)
	}

	return handler
}
