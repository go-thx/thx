package public

import (
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-thx/thx"
	"thx.test/gen/routes"
)

const (
	authCookieName = "auth"

	ParamPath = "path" // must match the path field in loginParams
)

var (
	scope = thx.Scope("public")

	idLoginForm  = scope("login-form")
	idForgotForm = scope("forgot-form")
	idResetForm  = scope("reset-form")
)

type Controller struct{}

//go:autowire
func New() *Controller {
	return &Controller{}
}

func (c *Controller) Routes() thx.Routes {
	return thx.Routes{
		thx.WithPath("/login", thx.WithLayout(layout, thx.Routes{
			thx.Get("/", c.getLogin),
			thx.Post("/", c.postLogin),
		})),

		thx.Get("/logout", c.getLogout),

		thx.HandleNotFound(func(ctx thx.Context) templ.Component {
			return notFound()
		}),
	}
}

func (c *Controller) getLogin(ctx thx.Context, query loginQuery) thx.Result {
	if ctx.IsAuthorized() {
		return ctx.Redirect(routes.Private().GetIndex().Path())
	}

	props := loginProps{
		form: loginForm{
			Path: query.Path,
		},
	}

	return thx.Render(ctx, loginView(props))
}

func (c *Controller) postLogin(ctx thx.Context, _ struct{}, form loginForm) thx.Result {
	ctx.SetCookie(authCookieName, "logged-in", time.Hour, false)

	redirectURL := form.Path

	if strings.TrimSpace(redirectURL) == "" {
		redirectURL = "/"
	}

	return ctx.Redirect(redirectURL)
}

func (c *Controller) getLogout(ctx thx.Context, _ struct{}) thx.Result {
	return ctx.DelCookie(authCookieName).Redirect("/")
}
