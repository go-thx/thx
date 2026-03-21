package internal

import (
	"context"
	"encoding/json"
	"fmt"
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

	SetValue(key, val any)

	WithLayouts() Context
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
		Path:     "/",
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
		Path:   "/",
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

func (c *contextImpl) SetValue(key, val any) {
	c.Context = context.WithValue(c.Context, key, val)
}

func (c *contextImpl) WithLayouts() Context {
	c.noLayouts = false
	return c
}

func (c *contextImpl) WithoutLayouts() Context {
	c.noLayouts = true
	return c
}

func (c *contextImpl) IsWithoutLayouts() bool {
	return c.noLayouts
}

func (c *contextImpl) HTMX() HTMX {
	return &htmx{
		req: c.req,
		res: c.res,
	}
}

// Swap strategies for HX-Reswap / hx-swap.
const (
	SwapInnerHTML   = "innerHTML"
	SwapOuterHTML   = "outerHTML"
	SwapBeforeBegin = "beforebegin"
	SwapAfterBegin  = "afterbegin"
	SwapBeforeEnd   = "beforeend"
	SwapAfterEnd    = "afterend"
	SwapDelete      = "delete"
	SwapNone        = "none"
	SwapInnerMorph  = "innerMorph"
	SwapOuterMorph  = "outerMorph"
	SwapTextContent = "textContent"
)

// StatusStopPolling is the HTTP status code (286) that tells HTMX to stop
// polling when using hx-trigger="every Xs".
const StatusStopPolling = 286

// LocationOptions configures a client-side HTMX navigation via HX-Location.
type LocationOptions struct {
	Path    string `json:"path"`
	Source  string `json:"source,omitempty"`
	Event   string `json:"event,omitempty"`
	Handler string `json:"handler,omitempty"`
	Target  string `json:"target,omitempty"`
	Swap    string `json:"swap,omitempty"`
	Values  any    `json:"values,omitempty"`
	Headers any    `json:"headers,omitempty"`
	Select  string `json:"select,omitempty"`
}

type HTMX interface {
	IsRequest() bool
	IsPartial() bool
	IsBoosted() bool
	CurrentURL() string
	IsHistoryRestoreRequest() bool
	Source() string
	Target() string

	Location(url string)
	LocationWithOptions(opts LocationOptions) error
	PushURL(url string)
	Redirect(url string)
	Refresh()
	ReplaceURL(url string)
	Reswap(strategy string)
	Retarget(selector string)
	Reselect(selector string)
	StopPolling()
	Trigger(event string, data any)
}

type htmx struct {
	req *http.Request
	res http.ResponseWriter
}

func (hx *htmx) IsRequest() bool {
	return hx.req.Header.Get("HX-Request") == "true"
}

func (hx *htmx) IsPartial() bool {
	return hx.req.Header.Get("HX-Request-Type") == "partial"
}

func (hx *htmx) IsBoosted() bool {
	return hx.req.Header.Get("HX-Boosted") == "true"
}

func (hx *htmx) CurrentURL() string {
	return hx.req.Header.Get("HX-Current-URL")
}

func (hx *htmx) IsHistoryRestoreRequest() bool {
	return hx.req.Header.Get("HX-History-Restore-Request") == "true"
}

func (hx *htmx) Source() string {
	return hx.req.Header.Get("HX-Source")
}

func (hx *htmx) Target() string {
	return hx.req.Header.Get("HX-Target")
}

func (hx *htmx) Location(url string) {
	hx.res.Header().Set("HX-Location", url)
}

func (hx *htmx) LocationWithOptions(opts LocationOptions) error {
	b, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("failed to marshal HX-Location options: %w", err)
	}
	hx.res.Header().Set("HX-Location", string(b))
	return nil
}

func (hx *htmx) PushURL(url string) {
	hx.res.Header().Set("HX-Push-Url", url)
}

func (hx *htmx) Redirect(url string) {
	hx.res.Header().Set("HX-Redirect", url)
}

func (hx *htmx) Refresh() {
	hx.res.Header().Set("HX-Refresh", "true")
}

func (hx *htmx) ReplaceURL(url string) {
	hx.res.Header().Set("HX-Replace-Url", url)
}

func (hx *htmx) Reswap(strategy string) {
	hx.res.Header().Set("HX-Reswap", strategy)
}

func (hx *htmx) Retarget(selector string) {
	hx.res.Header().Set("HX-Retarget", selector)
}

func (hx *htmx) Reselect(selector string) {
	hx.res.Header().Set("HX-Reselect", selector)
}

func (hx *htmx) StopPolling() {
	hx.res.WriteHeader(StatusStopPolling)
}

func (hx *htmx) Trigger(event string, data any) {
	setTriggerHeader(hx.res, "HX-Trigger", event, data)
}

func setTriggerHeader(res http.ResponseWriter, header, event string, data any) {
	if data == nil {
		existing := res.Header().Get(header)
		if existing == "" {
			res.Header().Set(header, event)
			return
		}

		// Existing value might be plain string or JSON — normalize to JSON.
		merged := parseTriggerHeader(existing)
		merged[event] = ""
		b, err := json.Marshal(merged)
		if err != nil {
			res.Header().Set(header, event)
			return
		}

		res.Header().Set(header, string(b))
		return
	}

	existing := res.Header().Get(header)
	merged := parseTriggerHeader(existing)
	merged[event] = data
	b, err := json.Marshal(merged)
	if err != nil {
		res.Header().Set(header, event)
		return
	}

	res.Header().Set(header, string(b))
}

func parseTriggerHeader(value string) map[string]any {
	if value == "" {
		return make(map[string]any)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(value), &m); err != nil {
		// Plain event name string — convert to map.
		return map[string]any{value: ""}
	}

	return m
}

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
