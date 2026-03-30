package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// contextKeyThx is the context key for storing the thx Context itself.
type contextKeyThx struct{}

// ViewContext retrieves the thx Context from a standard context.Context.
// This is needed because templ wraps contexts with context.WithValue,
// making direct type assertions fail.
// Panics if no thx Context is found — this indicates ViewContext was called
// outside of a thx request lifecycle (e.g. in a cached component rendered
// with context.Background).
func ViewContext(ctx context.Context) Context {
	if c, ok := ctx.(Context); ok {
		return c
	}

	if c, ok := ctx.Value(contextKeyThx{}).(Context); ok {
		return c
	}

	panic("thx: ViewContext called with a context that has no thx Context — ensure this is called within a thx request handler")
}

// Compile-time type guard.
var _ Context = (*contextImpl)(nil)

// Context is the request-scoped interface passed to all thx handlers.
// It extends context.Context with HTTP helpers for the current request.
type Context interface {
	context.Context

	// URI returns the raw request URI.
	URI() string
	// Header returns the value of the named request header.
	Header(key string) string
	// Param returns the value of a path parameter (e.g. {id}).
	Param(key string) string

	// Cookie returns the value of the named cookie, or "" if absent.
	Cookie(name string) string
	// SetCookie sets an HttpOnly cookie with the given name, value, and max age.
	SetCookie(name, value string, maxAge time.Duration, secure bool)
	// DelCookie deletes the named cookie by setting MaxAge to -1.
	DelCookie(name string) Context

	// FormFile returns the first file for the given form field name.
	FormFile(name string) (multipart.File, *multipart.FileHeader, error)

	// SetStatus writes the HTTP status code to the response.
	SetStatus(status int)
	// Redirect sends a redirect response. For HTMX requests, it sets
	// the HX-Redirect header instead of a standard HTTP redirect.
	Redirect(url string) Result

	// IsAuthorized returns true if an auth entity has been set on the context.
	IsAuthorized() bool

	// HTMX returns the HTMX sub-interface for reading request headers
	// and writing response headers.
	HTMX() HTMX

	// SetValue stores a key-value pair in the context.
	SetValue(key, val any)

	// WithLayouts re-enables layout rendering for this request.
	WithLayouts() Context
	// WithoutLayouts disables layout rendering for this request.
	WithoutLayouts() Context
	// IsWithoutLayouts returns true if layouts have been disabled.
	IsWithoutLayouts() bool
}

type contextImpl struct {
	context.Context //nolint:containedctx // embedded context is allowed here

	req *http.Request
	res http.ResponseWriter

	noLayouts bool
}

// NewContext creates a new request-scoped context and stores itself
// as a value for retrieval via ViewContext.
func NewContext(req *http.Request, res http.ResponseWriter) *contextImpl {
	c := &contextImpl{
		Context: req.Context(),
		req:     req,
		res:     res,
	}
	c.Context = context.WithValue(c.Context, contextKeyThx{}, c)
	return c
}

// URI returns the raw request URI string.
func (c *contextImpl) URI() string {
	return c.req.RequestURI
}

// Header returns the value of the named request header.
func (c *contextImpl) Header(key string) string {
	return c.req.Header.Get(key)
}

// Param returns the value of the named path parameter.
func (c *contextImpl) Param(key string) string {
	return c.req.PathValue(key)
}

// Cookie returns the value of the named cookie, or "" if absent or expired.
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

// FormFile returns the first file for the given form field name.
func (c *contextImpl) FormFile(name string) (multipart.File, *multipart.FileHeader, error) {
	return c.req.FormFile(name)
}

// SetCookie sets an HttpOnly, SameSite=Lax cookie with the given parameters.
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

// DelCookie deletes the named cookie by setting MaxAge to -1.
func (c *contextImpl) DelCookie(name string) Context {
	cookie := &http.Cookie{
		Name:   name,
		Path:   "/",
		MaxAge: -1,
	}

	http.SetCookie(c.res, cookie)

	return c
}

// SetStatus writes the HTTP status code header.
func (c *contextImpl) SetStatus(status int) {
	c.res.WriteHeader(status)
}

// Redirect sends a redirect. For HTMX requests it uses HX-Redirect;
// for standard requests it uses a 303 See Other redirect.
func (c *contextImpl) Redirect(url string) Result {
	if c.req.Header.Get("HX-Request") == "true" {
		c.res.Header().Set("HX-Redirect", url)
		return &EmptyResult{}
	}

	http.Redirect(c.res, c.req, url, http.StatusSeeOther)
	return &EmptyResult{}
}

// IsAuthorized returns true if an auth entity has been set via SetAuth.
func (c *contextImpl) IsAuthorized() bool {
	return c.Value(contextKeyAuth{}) != nil
}

// SetValue stores a key-value pair in the context.
func (c *contextImpl) SetValue(key, val any) {
	c.Context = context.WithValue(c.Context, key, val)
}

// WithLayouts re-enables layout rendering for this request.
func (c *contextImpl) WithLayouts() Context {
	c.noLayouts = false
	return c
}

// WithoutLayouts disables layout rendering for this request.
func (c *contextImpl) WithoutLayouts() Context {
	c.noLayouts = true
	return c
}

// IsWithoutLayouts returns true if layouts have been disabled.
func (c *contextImpl) IsWithoutLayouts() bool {
	return c.noLayouts
}

// HTMX returns the HTMX sub-interface for this request.
func (c *contextImpl) HTMX() HTMX {
	return &htmx{
		req: c.req,
		res: c.res,
	}
}

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

// HTMX provides access to HTMX request headers and response header setters.
type HTMX interface {
	// IsRequest returns true if this is an HTMX request (HX-Request: true).
	IsRequest() bool
	// IsPartial returns true if this is a partial HTMX request (not boosted).
	IsPartial() bool
	// IsBoosted returns true if this is an HTMX boosted request.
	IsBoosted() bool
	// CurrentURL returns the HX-Current-URL header value.
	CurrentURL() string
	// IsHistoryRestoreRequest returns true if HTMX is restoring history.
	IsHistoryRestoreRequest() bool
	// Source returns the ID of the element that triggered the request.
	Source() string
	// Target returns the ID of the target element for the response.
	Target() string

	// Location triggers a client-side navigation via HX-Location.
	Location(url string)
	// LocationWithOptions triggers a client-side navigation with fine-grained options.
	LocationWithOptions(opts LocationOptions) error
	// PushURL pushes the given URL onto the browser's history stack.
	PushURL(url string)
	// PreventPushURL prevents HTMX from pushing a URL to the history stack.
	PreventPushURL()
	// Redirect triggers a full-page redirect via HX-Redirect.
	Redirect(url string)
	// Refresh triggers a full page refresh via HX-Refresh.
	Refresh()
	// ReplaceURL replaces the current URL in the browser's location bar.
	ReplaceURL(url string)
	// PreventReplaceURL prevents HTMX from replacing the current URL.
	PreventReplaceURL()
	// Reswap overrides the swap strategy via HX-Reswap and returns a
	// SwapChain for adding modifiers like delays and scroll behavior.
	Reswap(strategy string) *SwapChain
	// Retarget overrides the target element via HX-Retarget.
	Retarget(selector string)
	// Reselect overrides which part of the response to swap via HX-Reselect.
	Reselect(selector string)
	// StopPolling sends HTTP 286 to stop HTMX polling.
	StopPolling()
	// Trigger fires a client-side event via HX-Trigger.
	// Returns a TriggerChain for adding multiple events.
	Trigger(event string, data any) *TriggerChain
}

type htmx struct {
	req *http.Request
	res http.ResponseWriter
}

// IsRequest checks the HX-Request header.
func (hx *htmx) IsRequest() bool {
	return hx.req.Header.Get("HX-Request") == "true"
}

// IsPartial checks the HX-Request-Type header for "partial".
func (hx *htmx) IsPartial() bool {
	return hx.req.Header.Get("HX-Request-Type") == "partial"
}

// IsBoosted checks the HX-Boosted header.
func (hx *htmx) IsBoosted() bool {
	return hx.req.Header.Get("HX-Boosted") == "true"
}

// CurrentURL returns the browser's current URL from HX-Current-URL.
func (hx *htmx) CurrentURL() string {
	return hx.req.Header.Get("HX-Current-URL")
}

// IsHistoryRestoreRequest checks the HX-History-Restore-Request header.
func (hx *htmx) IsHistoryRestoreRequest() bool {
	return hx.req.Header.Get("HX-History-Restore-Request") == "true"
}

// Source returns the HX-Source header value.
func (hx *htmx) Source() string {
	return hx.req.Header.Get("HX-Source")
}

// Target returns the HX-Target header value.
func (hx *htmx) Target() string {
	return hx.req.Header.Get("HX-Target")
}

// Location sets the HX-Location header for client-side navigation.
func (hx *htmx) Location(url string) {
	hx.res.Header().Set("HX-Location", url)
}

// LocationWithOptions sets HX-Location with a JSON-encoded options object.
func (hx *htmx) LocationWithOptions(opts LocationOptions) error {
	b, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("failed to marshal HX-Location options: %w", err)
	}
	hx.res.Header().Set("HX-Location", string(b))
	return nil
}

// PushURL sets the HX-Push-Url header.
func (hx *htmx) PushURL(url string) {
	hx.res.Header().Set("HX-Push-Url", url)
}

// PreventPushURL sets HX-Push-Url to "false".
func (hx *htmx) PreventPushURL() {
	hx.res.Header().Set("HX-Push-Url", "false")
}

// Redirect sets the HX-Redirect header for a full-page redirect.
func (hx *htmx) Redirect(url string) {
	hx.res.Header().Set("HX-Redirect", url)
}

// Refresh sets HX-Refresh to "true" for a full page refresh.
func (hx *htmx) Refresh() {
	hx.res.Header().Set("HX-Refresh", "true")
}

// ReplaceURL sets the HX-Replace-Url header.
func (hx *htmx) ReplaceURL(url string) {
	hx.res.Header().Set("HX-Replace-Url", url)
}

// PreventReplaceURL sets HX-Replace-Url to "false".
func (hx *htmx) PreventReplaceURL() {
	hx.res.Header().Set("HX-Replace-Url", "false")
}

// Reswap sets HX-Reswap and returns a SwapChain for adding modifiers.
func (hx *htmx) Reswap(strategy string) *SwapChain {
	hx.res.Header().Set("HX-Reswap", strategy)
	return &SwapChain{res: hx.res, strategy: strategy}
}

// Retarget sets the HX-Retarget header.
func (hx *htmx) Retarget(selector string) {
	hx.res.Header().Set("HX-Retarget", selector)
}

// Reselect sets the HX-Reselect header.
func (hx *htmx) Reselect(selector string) {
	hx.res.Header().Set("HX-Reselect", selector)
}

// StopPolling writes HTTP 286 to tell HTMX to stop polling.
func (hx *htmx) StopPolling() {
	hx.res.WriteHeader(StatusStopPolling)
}

// Trigger sets HX-Trigger and returns a TriggerChain for adding more events.
func (hx *htmx) Trigger(event string, data any) *TriggerChain {
	tc := &TriggerChain{res: hx.res}
	return tc.Trigger(event, data)
}

// SwapChain provides fluent modifiers for the HX-Reswap header.
// Each method updates the header immediately. Calling the same modifier
// type twice replaces the previous value (e.g. ScrollTop then ScrollBottom
// keeps only ScrollBottom).
// swapModOrder defines the output order for swap modifiers.
var swapModOrder = []string{"swap", "settle", "transition", "ignoreTitle", "focus-scroll", "scroll", "show"}

type SwapChain struct {
	res      http.ResponseWriter
	strategy string
	mods     map[string]string
}

// set adds or replaces a swap modifier and updates the HX-Reswap header.
func (s *SwapChain) set(prefix, value string) *SwapChain {
	if s.mods == nil {
		s.mods = make(map[string]string)
	}
	s.mods[prefix] = value
	s.write()
	return s
}

// write rebuilds the HX-Reswap header from the strategy and all modifiers.
func (s *SwapChain) write() {
	val := s.strategy
	for _, key := range swapModOrder {
		if v, ok := s.mods[key]; ok {
			val += " " + v
		}
	}
	s.res.Header().Set("HX-Reswap", val)
}

// SwapDelay adds a swap delay modifier.
func (s *SwapChain) SwapDelay(d time.Duration) *SwapChain {
	return s.set("swap", "swap:"+d.String())
}

// SettleDelay adds a settle delay modifier.
func (s *SwapChain) SettleDelay(d time.Duration) *SwapChain {
	return s.set("settle", "settle:"+d.String())
}

// Transition enables view transitions for this swap.
func (s *SwapChain) Transition() *SwapChain {
	return s.set("transition", "transition:true")
}

// IgnoreTitle prevents HTMX from updating the page title.
func (s *SwapChain) IgnoreTitle() *SwapChain {
	return s.set("ignoreTitle", "ignoreTitle:true")
}

// FocusScroll enables or disables focus scrolling after swap.
func (s *SwapChain) FocusScroll(enabled bool) *SwapChain {
	return s.set("focus-scroll", fmt.Sprintf("focus-scroll:%t", enabled))
}

// ScrollTop scrolls to the top of the element matching the selector.
func (s *SwapChain) ScrollTop(selector string) *SwapChain {
	if selector == "" {
		return s.set("scroll", "scroll:top")
	}
	return s.set("scroll", fmt.Sprintf("scroll:%s:top", selector))
}

// ScrollBottom scrolls to the bottom of the element matching the selector.
func (s *SwapChain) ScrollBottom(selector string) *SwapChain {
	if selector == "" {
		return s.set("scroll", "scroll:bottom")
	}
	return s.set("scroll", fmt.Sprintf("scroll:%s:bottom", selector))
}

// ScrollWindowTop scrolls the window to the top.
func (s *SwapChain) ScrollWindowTop() *SwapChain {
	return s.set("scroll", "scroll:window:top")
}

// ScrollWindowBottom scrolls the window to the bottom.
func (s *SwapChain) ScrollWindowBottom() *SwapChain {
	return s.set("scroll", "scroll:window:bottom")
}

// ShowTop shows the top of the element matching the selector in the viewport.
func (s *SwapChain) ShowTop(selector string) *SwapChain {
	if selector == "" {
		return s.set("show", "show:top")
	}
	return s.set("show", fmt.Sprintf("show:%s:top", selector))
}

// ShowBottom shows the bottom of the element matching the selector in the viewport.
func (s *SwapChain) ShowBottom(selector string) *SwapChain {
	if selector == "" {
		return s.set("show", "show:bottom")
	}
	return s.set("show", fmt.Sprintf("show:%s:bottom", selector))
}

// ShowWindowTop scrolls the window to show the top.
func (s *SwapChain) ShowWindowTop() *SwapChain {
	return s.set("show", "show:window:top")
}

// ShowWindowBottom scrolls the window to show the bottom.
func (s *SwapChain) ShowWindowBottom() *SwapChain {
	return s.set("show", "show:window:bottom")
}

// ShowNone disables automatic scrolling after swap.
func (s *SwapChain) ShowNone() *SwapChain {
	return s.set("show", "show:none")
}

// TriggerChain provides fluent chaining for HX-Trigger events.
// Each method updates the header immediately.
type TriggerChain struct {
	res    http.ResponseWriter
	events map[string]any
}

// write updates the HX-Trigger header from the accumulated events.
func (t *TriggerChain) write() {
	if len(t.events) == 1 {
		for k, v := range t.events {
			if v == nil {
				t.res.Header().Set("HX-Trigger", k)
				return
			}
		}
	}
	b, err := json.Marshal(t.events)
	if err != nil {
		// Fall back to comma-separated plain event names.
		names := make([]string, 0, len(t.events))
		for k := range t.events {
			names = append(names, k)
		}
		t.res.Header().Set("HX-Trigger", strings.Join(names, ", "))
		return
	}
	t.res.Header().Set("HX-Trigger", string(b))
}

// Trigger adds an event to the HX-Trigger header. Pass nil for data
// to trigger an event with no payload.
func (t *TriggerChain) Trigger(event string, data any) *TriggerChain {
	if t.events == nil {
		t.events = parseTriggerHeader(t.res.Header().Get("HX-Trigger"))
	}
	if data == nil {
		t.events[event] = nil
	} else {
		t.events[event] = data
	}
	t.write()
	return t
}

// parseTriggerHeader parses an existing HX-Trigger header value,
// supporting both JSON object and comma-separated event name formats.
func parseTriggerHeader(value string) map[string]any {
	if value == "" {
		return make(map[string]any)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(value), &m); err == nil {
		return m
	}

	// Parse comma-separated plain event names.
	result := make(map[string]any)
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			result[name] = nil
		}
	}
	return result
}

type contextKeyAuth struct{}

// SetAuth stores the authenticated entity in the context for later
// retrieval via AuthContext[T].Auth().
func SetAuth[T any](ctx context.Context, auth T) context.Context {
	return context.WithValue(ctx, contextKeyAuth{}, auth)
}

// AuthContext extends Context with access to the authenticated entity of type T.
type AuthContext[T any] interface {
	Context

	// Auth returns the entity for the authenticated request.
	Auth() T
}

type authContextImpl[T any] struct {
	Context
}

// Auth returns the authenticated entity, or the zero value if not found.
func (c *authContextImpl[T]) Auth() T {
	if auth, ok := c.Value(contextKeyAuth{}).(T); ok {
		return auth
	}

	var t T
	return t
}

// NewAuthContext wraps an existing Context into an AuthContext.
func NewAuthContext[T any](ctx Context) AuthContext[T] {
	return &authContextImpl[T]{ctx}
}
