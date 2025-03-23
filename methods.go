package thx

import (
	"net/http"
)

var (
	methodGet = func(mux *http.ServeMux) func(pattern string, handlerFn http.HandlerFunc) {
		return func(pattern string, handlerFn http.HandlerFunc) {
			mux.HandleFunc("GET "+pattern, handlerFn)
		}
	}
	methodPost = func(mux *http.ServeMux) func(pattern string, handlerFn http.HandlerFunc) {
		return func(pattern string, handlerFn http.HandlerFunc) {
			mux.HandleFunc("POST "+pattern, handlerFn)
		}
	}
	methodPut = func(mux *http.ServeMux) func(pattern string, handlerFn http.HandlerFunc) {
		return func(pattern string, handlerFn http.HandlerFunc) {
			mux.HandleFunc("PUT "+pattern, handlerFn)
		}
	}
	methodPatch = func(mux *http.ServeMux) func(pattern string, handlerFn http.HandlerFunc) {
		return func(pattern string, handlerFn http.HandlerFunc) {
			mux.HandleFunc("PATCH "+pattern, handlerFn)
		}
	}
	methodDelete = func(mux *http.ServeMux) func(pattern string, handlerFn http.HandlerFunc) {
		return func(pattern string, handlerFn http.HandlerFunc) {
			mux.HandleFunc("DELETE "+pattern, handlerFn)
		}
	}
)

func Get[Q, O any](path string, handler GetHandler[Q, O]) Route {
	return &route[Q, struct{}, O]{
		method:     methodGet,
		path:       path,
		getHandler: handler,
	}
}

func Post[Q, I, O any](path string, handler Handler[Q, I, O]) Route {
	return &route[Q, I, O]{
		method:  methodPost,
		path:    path,
		handler: handler,
	}
}

func Put[Q, I, O any](path string, handler Handler[Q, I, O]) Route {
	return &route[Q, I, O]{
		method:  methodPut,
		path:    path,
		handler: handler,
	}
}

func Patch[Q, I, O any](path string, handler Handler[Q, I, O]) Route {
	return &route[Q, I, O]{
		method:  methodPatch,
		path:    path,
		handler: handler,
	}
}

func Delete[Q, I, O any](path string, handler Handler[Q, I, O]) Route {
	return &route[Q, I, O]{
		method:  methodDelete,
		path:    path,
		handler: handler,
	}
}
