package thx

import (
	"net/http"
)

type Route interface {
	Apply(*Router)
}

type Router struct {
	Mux *http.ServeMux

	Path    string
	Layouts []Layout
	ErrorHandler func(Context, http.ResponseWriter, *http.Request, error)

	NotFoundHandler func(http.ResponseWriter, *http.Request)
}

// New creates a new router with the given routes.
// It returns an *http.ServeMux that can be used to serve HTTP requests.
func New(routes ...Route) *http.ServeMux {
	mux := http.NewServeMux()

	router := &Router{
		Mux:             mux,
		NotFoundHandler: http.NotFound,
	}

	for _, route := range routes {
		route.Apply(router)
	}

	return mux
}
