package thx

import "net/http"

func Get[Q any](path string, handler GetHandler[Q]) Route {
	return &route[Q, struct{}]{
		method:     http.MethodGet,
		path:       path,
		getHandler: handler,
	}
}

func Post[Q, I any](path string, handler Handler[Q, I]) Route {
	return &route[Q, I]{
		method:  http.MethodPost,
		path:    path,
		handler: handler,
	}
}

func Put[Q, I any](path string, handler Handler[Q, I]) Route {
	return &route[Q, I]{
		method:  http.MethodPut,
		path:    path,
		handler: handler,
	}
}

func Patch[Q, I any](path string, handler Handler[Q, I]) Route {
	return &route[Q, I]{
		method:  http.MethodPatch,
		path:    path,
		handler: handler,
	}
}

func Delete[Q, I any](path string, handler Handler[Q, I]) Route {
	return &route[Q, I]{
		method:  http.MethodDelete,
		path:    path,
		handler: handler,
	}
}
