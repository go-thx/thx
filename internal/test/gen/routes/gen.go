package routes

import "path"

func GetIndex() Route {
	return Route{"/", "GET"}
}

func Public() public {
	return public{
		path: "/public",
	}
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

func Private() private {
	return private{
		path: "/private",
	}
}

type private struct {
	path string
}

func (p private) GetIndex() Route {
	return Route{path.Join(p.path, "/"), "GET"}
}

func (p private) Events() Route {
	return Route{path.Join(p.path, "/events"), "SSE"}
}

func (p private) Ws() Route {
	return Route{path.Join(p.path, "/ws"), "WS"}
}
