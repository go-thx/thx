package thx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-thx/thx/internal"
)

// Result is the return type of all route handlers.
// It writes the handler's output to the HTTP response.
type Result = internal.Result

type layoutsKey struct{}

// Empty returns a Result that writes nothing to the response.
func Empty() Result {
	return &internal.EmptyResult{}
}

// Render returns a Result that renders the templ component with
// all active layouts applied. This is the primary way to return HTML.
func Render(ctx Context, comp templ.Component) Result {
	return &renderResult{ctx: ctx, comp: comp}
}

// Partial returns a Result that renders the component without any layouts.
// Useful for returning HTML fragments for HTMX partial updates.
func Partial(ctx Context, comp templ.Component) Result {
	ctx.WithoutLayouts()
	return Render(ctx, comp)
}

// JSON returns a Result that serializes data as JSON with the
// appropriate Content-Type header.
func JSON(data any) Result {
	return &jsonResult{data: data}
}

// Raw returns a Result that writes the string as plain text.
func Raw(val string) Result {
	return &rawResult{val: val}
}

// Status begins building a Result with a custom HTTP status code.
// Chain with Render, JSON, Raw, or Empty to complete the result.
//
//	return thx.Status(http.StatusCreated).JSON(user)
func Status(code int) *StatusBuilder {
	return &StatusBuilder{code: code}
}

// StatusBuilder constructs results with a specific HTTP status code.
type StatusBuilder struct {
	code int
}

// Render returns a Result that renders the component with the configured status code.
func (s *StatusBuilder) Render(ctx Context, comp templ.Component) Result {
	return &renderResult{ctx: ctx, comp: comp, status: s.code}
}

// JSON returns a Result that serializes data as JSON with the configured status code.
func (s *StatusBuilder) JSON(data any) Result {
	return &jsonResult{data: data, status: s.code}
}

// Raw returns a Result that writes the string as plain text with the configured status code.
func (s *StatusBuilder) Raw(val string) Result {
	return &rawResult{val: val, status: s.code}
}

// Empty returns a Result that writes only the configured status code with no body.
func (s *StatusBuilder) Empty() Result {
	return &statusResult{code: s.code}
}

// renderResult renders a templ component with layout wrapping.
type renderResult struct {
	ctx    Context
	comp   templ.Component
	status int
}

// WriteResult applies layouts to the component and renders it to the response.
func (r *renderResult) WriteResult(res http.ResponseWriter) error {
	comp := r.comp
	if layouts, ok := r.ctx.Value(layoutsKey{}).([]Layout); ok {
		comp = applyLayouts(r.ctx, comp, layouts)
	}
	if r.status > 0 {
		res.WriteHeader(r.status)
	}
	return comp.Render(r.ctx, res)
}

// jsonResult serializes data as JSON.
type jsonResult struct {
	data   any
	status int
}

// WriteResult sets the JSON content type and encodes the data.
func (r *jsonResult) WriteResult(res http.ResponseWriter) error {
	res.Header().Set("Content-Type", "application/json")
	if r.status > 0 {
		res.WriteHeader(r.status)
	}
	return json.NewEncoder(res).Encode(r.data)
}

// rawResult writes a plain text string.
type rawResult struct {
	val    string
	status int
}

// WriteResult sets the text/plain content type and writes the string.
func (r *rawResult) WriteResult(res http.ResponseWriter) error {
	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.status > 0 {
		res.WriteHeader(r.status)
	}
	_, err := fmt.Fprint(res, r.val)
	return err
}

// statusResult writes only an HTTP status code with no body.
type statusResult struct {
	code int
}

// WriteResult writes the status code header.
func (s *statusResult) WriteResult(res http.ResponseWriter) error {
	res.WriteHeader(s.code)
	return nil
}

// ViewContext retrieves the thx Context from a standard context.Context.
// Use this inside templ components to access request-scoped helpers like
// HTMX(), Param(), or Cookie(). Panics if called outside a thx request.
func ViewContext(ctx context.Context) Context {
	return internal.ViewContext(ctx)
}
