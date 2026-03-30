package thx

import "net/http"

// Get registers a GET route at the given path. Q is the query parameter type.
// The handler receives the decoded query parameters and returns a Result.
func Get[Q any](path string, handler GetHandler[Q]) Route {
	return &route[Q, struct{}]{
		method:     http.MethodGet,
		path:       path,
		getHandler: handler,
	}
}

// Post registers a POST route at the given path. Q is the query parameter
// type and I is the request body input type (form or JSON).
func Post[Q, I any](path string, handler Handler[Q, I]) Route {
	return &route[Q, I]{
		method:  http.MethodPost,
		path:    path,
		handler: handler,
	}
}

// Put registers a PUT route at the given path. Q is the query parameter
// type and I is the request body input type.
func Put[Q, I any](path string, handler Handler[Q, I]) Route {
	return &route[Q, I]{
		method:  http.MethodPut,
		path:    path,
		handler: handler,
	}
}

// Patch registers a PATCH route at the given path. Q is the query parameter
// type and I is the request body input type.
func Patch[Q, I any](path string, handler Handler[Q, I]) Route {
	return &route[Q, I]{
		method:  http.MethodPatch,
		path:    path,
		handler: handler,
	}
}

// Delete registers a DELETE route at the given path. Q is the query parameter
// type and I is the request body input type.
func Delete[Q, I any](path string, handler Handler[Q, I]) Route {
	return &route[Q, I]{
		method:  http.MethodDelete,
		path:    path,
		handler: handler,
	}
}
