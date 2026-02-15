package routes

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
