package internal

import (
	"context"
	"net/http"
	"time"
)

// contextKeyThx is the context key for storing the thx Context itself.
type contextKeyThx struct{}

// FromContext retrieves the thx Context from a standard context.Context.
// This is needed because templ wraps contexts with context.WithValue,
// making direct type assertions fail.
func FromContext(ctx context.Context) Context {
	if c, ok := ctx.(Context); ok {
		return c
	}

	if c, ok := ctx.Value(contextKeyThx{}).(Context); ok {
		return c
	}

	return nil
}

// Compile-time type guard.
var _ Context = (*contextImpl)(nil)

type Context interface {
	context.Context

	URI() string
	Header(key string) string
	Param(key string) string

	Cookie(name string) string
	SetCookie(name, value string, maxAge time.Duration, secure bool)
	DelCookie(name string) Context

	SetStatus(status int)
	Redirect(url string) View

	IsAuthorized() bool

	HTMX() HTMX

	WithoutLayouts() Context
	IsWithoutLayouts() bool
}

type contextImpl struct {
	context.Context //nolint:containedctx // embedded context is allowed here

	req *http.Request
	res http.ResponseWriter

	noLayouts bool
}

func NewContext(req *http.Request, res http.ResponseWriter) *contextImpl {
	c := &contextImpl{
		Context: req.Context(),
		req:     req,
		res:     res,
	}
	c.Context = context.WithValue(c.Context, contextKeyThx{}, c)
	return c
}

func (c *contextImpl) URI() string {
	return c.req.RequestURI
}

func (c *contextImpl) Header(key string) string {
	return c.req.Header.Get(key)
}

func (c *contextImpl) Param(key string) string {
	return c.req.PathValue(key)
}

func (c *contextImpl) Cookie(name string) string {
	cookie, err := c.req.Cookie(name)
	if err != nil {
		return ""
	}

	if cookie.MaxAge < 0 {
		return ""
	}

	return cookie.Value
}

func (c *contextImpl) SetCookie(name, value string, maxAge time.Duration, secure bool) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // lax mode allows authenticated GET requests (linking) from external sites
		Secure:   secure,
	}

	if maxAge > 0 {
		cookie.MaxAge = int(maxAge.Seconds())
	}

	http.SetCookie(c.res, cookie)
}

func (c *contextImpl) DelCookie(name string) Context {
	cookie := &http.Cookie{
		Name:   name,
		MaxAge: -1,
	}

	http.SetCookie(c.res, cookie)

	return c
}

func (c *contextImpl) SetStatus(status int) {
	c.res.WriteHeader(status)
}

func (c *contextImpl) Redirect(url string) View {
	if c.req.Header.Get("HX-Request") == "true" {
		c.res.Header().Set("HX-Redirect", url)
		return Empty()
	}

	http.Redirect(c.res, c.req, url, http.StatusSeeOther)
	return Empty()
}

func (c *contextImpl) IsAuthorized() bool {
	return c.Value(contextKeyAuth{}) != nil
}

// TEMPL

func (c *contextImpl) WithoutLayouts() Context {
	c.noLayouts = true
	return c
}

func (c *contextImpl) IsWithoutLayouts() bool {
	return c.noLayouts
}

// HTMX

func (c *contextImpl) HTMX() HTMX {
	return &htmx{
		req: c.req,
		res: c.res,
	}
}

type HTMX interface {
	IsRequest() bool
	IsBoosted() bool
	PushURL(url string)
	ReplaceURL(url string)
	Trigger(event string, data any)
	Retarget(target string)
}

type htmx struct {
	req *http.Request
	res http.ResponseWriter
}

func (hx *htmx) IsRequest() bool {
	return hx.req.Header.Get("HX-Request") == "true"
}

func (hx *htmx) IsBoosted() bool {
	return hx.req.Header.Get("HX-Boosted") == "true"
}

func (hx *htmx) PushURL(url string) {
	hx.res.Header().Set("HX-Push-Url", url)
}

func (hx *htmx) ReplaceURL(url string) {
	hx.res.Header().Set("HX-Replace-Url", url)
}

func (hx *htmx) Trigger(event string, data any) {
	// TODO: https://vimperium.studio/articles/htmx-notifications
	hx.res.Header().Set("HX-Trigger", event)
}

func (hx *htmx) Retarget(target string) {
	hx.res.Header().Set("HX-Retarget", target)
}

//
// -- AUTH
//

// contextKeyAuth is the context key for storing authenticated user/entity
type contextKeyAuth struct{}

func SetAuth[T any](ctx context.Context, auth T) context.Context {
	return context.WithValue(ctx, contextKeyAuth{}, auth)
}

type AuthContext[T any] interface {
	Context

	// Auth returns the entity for the authenticated request.
	Auth() T
}

type authContextImpl[T any] struct {
	Context
}

func (c *authContextImpl[T]) Auth() T {
	if auth, ok := c.Value(contextKeyAuth{}).(T); ok {
		return auth
	}

	var t T
	return t
}

func NewAuthContext[T any](ctx Context) AuthContext[T] {
	return &authContextImpl[T]{ctx}
}
