package public

type loginQuery struct {
	Path string `schema:"path"`
}

type loginForm struct {
	Email    string `schema:"email"`
	Password string `schema:"password"` //nolint:gosec // form binding, immediately hashed
	Path     string `schema:"path"`
}

type loginProps struct {
	form  loginForm
	error string
}
