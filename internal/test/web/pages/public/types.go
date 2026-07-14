package public

type loginQuery struct {
	Path string `thx:"path"`
}

type loginForm struct {
	Email    string `thx:"email"`
	Password string `thx:"password"` //nolint:gosec // form binding, immediately hashed
	Path     string `thx:"path"`
}

type loginProps struct {
	form  loginForm
	error string
}
