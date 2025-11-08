package thx

import "net/http"

func Get[Q, O any](path string, handler GetHandler[Q, O]) Route {
	return &route[Q, struct{}, O]{
		method:     http.MethodGet,
		path:       path,
		getHandler: handler,
	}
}

func Post[Q, I, O any](path string, handler Handler[Q, I, O]) Route {
	return &route[Q, I, O]{
		method:  http.MethodPost,
		path:    path,
		handler: handler,
	}
}

func Put[Q, I, O any](path string, handler Handler[Q, I, O]) Route {
	return &route[Q, I, O]{
		method:  http.MethodPut,
		path:    path,
		handler: handler,
	}
}

func Patch[Q, I, O any](path string, handler Handler[Q, I, O]) Route {
	return &route[Q, I, O]{
		method:  http.MethodPatch,
		path:    path,
		handler: handler,
	}
}

func Delete[Q, I, O any](path string, handler Handler[Q, I, O]) Route {
	return &route[Q, I, O]{
		method:  http.MethodDelete,
		path:    path,
		handler: handler,
	}
}