package thx

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-thx/thx/internal"
)

type View = internal.View

func Empty() View {
	return internal.Empty()
}

func Render(ctx Context, comp templ.Component) View {
	return &baseView{ctx, comp}
}

type baseView struct {
	ctx  Context
	comp templ.Component
}

func (v *baseView) Render(ctx context.Context, w io.Writer) error {
	return v.comp.Render(ctx, w)
}

func (v *baseView) Out(_ context.Context, w http.ResponseWriter) error {
	return v.comp.Render(v.ctx, w)
}

func Partial(ctx internal.Context, comp templ.Component) View {
	ctx.WithoutLayouts()
	return Render(ctx, comp)
}

func BadRequest() View {
	return &errorView{
		status:  http.StatusBadRequest,
		message: http.StatusText(http.StatusBadRequest),
	}
}

func Error(err error) View {
	slog.Error("Controller returned an error.",
		"error", err,
	)

	return &errorView{
		status:  http.StatusInternalServerError,
		message: http.StatusText(http.StatusInternalServerError),
	}
}

type errorView struct {
	status  int
	message string
}

func (e *errorView) Out(_ context.Context, w http.ResponseWriter) error {
	http.Error(w, e.message, e.status)

	return nil
}

func ViewContext(ctx context.Context) Context {
	return internal.FromContext(ctx)
}
