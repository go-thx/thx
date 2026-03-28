package thx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-thx/thx/internal"
)

type Result = internal.Result

type layoutsKey struct{}

func Empty() Result {
	return &internal.EmptyResult{}
}

func Render(ctx Context, comp templ.Component) Result {
	return &renderResult{ctx: ctx, comp: comp}
}

func Partial(ctx Context, comp templ.Component) Result {
	ctx.WithoutLayouts()
	return Render(ctx, comp)
}

func JSON(data any) Result {
	return &jsonResult{data: data}
}

func Raw(val string) Result {
	return &rawResult{val: val}
}

func Status(code int) *StatusBuilder {
	return &StatusBuilder{code: code}
}

type StatusBuilder struct {
	code int
}

func (s *StatusBuilder) Render(ctx Context, comp templ.Component) Result {
	return &renderResult{ctx: ctx, comp: comp, status: s.code}
}

func (s *StatusBuilder) JSON(data any) Result {
	return &jsonResult{data: data, status: s.code}
}

func (s *StatusBuilder) Raw(val string) Result {
	return &rawResult{val: val, status: s.code}
}

func (s *StatusBuilder) Empty() Result {
	return &statusResult{code: s.code}
}

type renderResult struct {
	ctx    Context
	comp   templ.Component
	status int
}

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

type jsonResult struct {
	data   any
	status int
}

func (r *jsonResult) WriteResult(res http.ResponseWriter) error {
	res.Header().Set("Content-Type", "application/json")
	if r.status > 0 {
		res.WriteHeader(r.status)
	}
	return json.NewEncoder(res).Encode(r.data)
}

type rawResult struct {
	val    string
	status int
}

func (r *rawResult) WriteResult(res http.ResponseWriter) error {
	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.status > 0 {
		res.WriteHeader(r.status)
	}
	_, err := fmt.Fprint(res, r.val)
	return err
}

type statusResult struct {
	code int
}

func (s *statusResult) WriteResult(res http.ResponseWriter) error {
	http.Error(res, http.StatusText(s.code), s.code)
	return nil
}

func ViewContext(ctx context.Context) Context {
	return internal.FromContext(ctx)
}
