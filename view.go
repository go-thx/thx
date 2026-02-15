package thx

import (
	"context"
	"io"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-thx/thx/internal"
)

type View = internal.View

func Empty() View {
	return internal.Empty()
}

func Render(ctx Context, comp templ.Component) View {
	return &baseView{ctx: ctx, comp: comp}
}

type baseView struct {
	ctx  Context
	comp templ.Component
}

func (v *baseView) Render(_ context.Context, w io.Writer) error {
	return v.comp.Render(v.ctx, w)
}

func (v *baseView) Out(_ context.Context, w http.ResponseWriter) error {
	return v.comp.Render(v.ctx, w)
}

func Partial(ctx Context, comp templ.Component) View {
	ctx.WithoutLayouts()
	return Render(ctx, comp)
}

func Status(code int) View {
	return &statusView{code: code}
}

type statusView struct {
	code int
}

func (s *statusView) Out(_ context.Context, w http.ResponseWriter) error {
	http.Error(w, http.StatusText(s.code), s.code)
	return nil
}

func ViewContext(ctx context.Context) Context {
	return internal.FromContext(ctx)
}