package routes

import "path"

type Route struct {
	path   string
	method string
}

func (r Route) Path() string {
	return r.path
}

func (r Route) Method() string {
	return r.method
}

func Public() public {
	return public{}
}

type public struct {
	path string
}

func (p public) GetLogin() Route {
	return Route{path.Join(p.path, "/login"), "GET"}
}

func (p public) PostLogin() Route {
	return Route{path.Join(p.path, "/login"), "POST"}
}

func (p public) GetLogout() Route {
	return Route{path.Join(p.path, "/logout"), "GET"}
}

type private struct {
	path string
}

func (p private) Index() Route {
	return Route{path.Join(p.path, "/"), "GET"}
}
