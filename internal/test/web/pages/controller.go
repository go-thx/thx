package pages

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-thx/thx"
	thxauth "github.com/go-thx/thx/auth"
	"thx.test/gen/assets"
	"thx.test/gen/routes"
	"thx.test/model"
	"thx.test/web/mw"
	"thx.test/web/pages/private"
	"thx.test/web/pages/public"
)

// csrfKey is the HMAC key for CSRF token generation.
// In a real application, load this from configuration or environment.
var csrfKey = []byte("thx-test-csrf-key-change-me")

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
		thx.Static("/assets", assets.Assets()),

		thx.WithMiddleware(
			thx.Chain(
				mw.RequestID,
				c.logger,
				mw.CSRF(csrfKey),
				mw.Nonce,
				authMiddleware,
			),
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
		),
	}
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		cookie, err := req.Cookie("auth")

		if err == nil && cookie.Value == "logged-in" {
			req = req.WithContext(thx.SetAuth(req.Context(), model.User{1, "User"}))
		}

		next.ServeHTTP(res, req)
	})
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
