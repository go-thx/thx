package private

import (
	"github.com/go-thx/thx"
	"github.com/go-thx/thx/thxauth"
	"thx.test/web/auth"
)

type Controller struct{}

func New() *Controller {
	return &Controller{}
}

func (c *Controller) Routes() []thx.Route {
	return thx.WithLayout(layout,
		thx.Routes{
			thx.Get("/", thxauth.Get(c.index)),
		},
	)
}

func (c *Controller) index(ctx auth.Context, _ struct{}) thx.View {
	return thx.Render(ctx, index())
}
