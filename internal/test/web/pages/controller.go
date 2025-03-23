package pages

import (
	"github.com/go-thx/thx"
	"github.com/go-thx/thx/thxauth"
	"thx.test/gen/routes"
	"thx.test/web/pages/private"
	"thx.test/web/pages/public"
)

type Controller struct {
	public  *public.Controller
	private *private.Controller
}

func New(
	public *public.Controller,
	private *private.Controller,
) *Controller {
	return &Controller{
		public,
		private,
	}
}

func (c *Controller) Routes() thx.Routes {
	return thx.Routes{
		thx.WithLayout(baseLayout, c.public.Routes()),
		thxauth.Guard(
			thx.WithLayout(baseLayout, c.private.Routes()),
			thxauth.RedirectUnauthorized(routes.Public().GetLogin().Path()),
			thxauth.RedirectWithCurrentPath(public.ParamPath),
		),
	}
}
