package public

import (
	"strings"
	"time"

	"github.com/go-thx/thx"
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
	}
}

func (c *Controller) getLogin(ctx thx.Context, query loginQuery) thx.View {
	if ctx.IsAuthorized() {
		return ctx.Redirect("/") // webpath.Dashboard().Get()
	}

	props := loginProps{
		form: loginForm{
			Path: query.Path,
		},
	}

	return thx.Render(ctx, loginView(props))
}

func (c *Controller) postLogin(ctx thx.Context, _ struct{}, form loginForm) thx.View {
	ctx.SetCookie(authCookieName, "logged-in", time.Hour, false)

	redirectURL := form.Path

	if strings.TrimSpace(redirectURL) == "" {
		redirectURL = "/"
	}

	return ctx.Redirect(redirectURL)
}

func (c *Controller) getLogout(ctx thx.Context, _ struct{}) thx.View {
	return ctx.DelCookie(authCookieName).Redirect("/")
}
