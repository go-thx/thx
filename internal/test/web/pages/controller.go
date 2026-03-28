package pages

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-thx/thx"
	thxauth "github.com/go-thx/thx/auth"
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
	return thx.WithMiddleware(
		thx.Chain(c.logger),
		thx.WithLayout(baseLayout,
			thx.Get("/", c.getIndex),

			thx.WithPath("/public", c.public.Routes()),

			thxauth.WithGuard("/private",
				c.private.Routes(),
				thxauth.RedirectUnauthorized(routes.Public().GetLogin().Path()),
				thxauth.RedirectWithCurrentPath(public.ParamPath),
			),

			thx.HandleNotFound(func(ctx thx.Context) templ.Component {
				return notFound()
			}),
		),
	)
}

func (c *Controller) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.InfoContext(r.Context(), "Request",
			"method", r.Method,
			"path", r.URL.Path,
			"host", r.Host,
			"remote", r.RemoteAddr,
			"user-agent", r.UserAgent(),
		)

		next.ServeHTTP(w, r)
	})
}

func (c *Controller) getIndex(ctx thx.Context, _ struct{}) thx.Result {
	return thx.Render(ctx, index())
}

/*

	thx.Get[any, any]("/{$}", func(context thx.Context, a any) any {
		return nil
	}),
*/
