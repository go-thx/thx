package thx

import (
	"net/http"
)

type Route interface {
	Apply(*Router)
}

type Router struct {
	Mux *http.ServeMux

	Path       string
	Layouts    []Layout
	Middleware func(http.Handler) http.Handler

	ErrorHandler    func(Context, http.ResponseWriter, *http.Request, error)
	NotFoundHandler func(http.ResponseWriter, *http.Request)
}

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
