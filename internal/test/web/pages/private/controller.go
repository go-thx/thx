package private

import (
	"fmt"
	"time"

	"net/http"

	"github.com/a-h/templ"
	"github.com/go-thx/thx"
	thxauth "github.com/go-thx/thx/auth"
	"thx.test/web/auth"
)

type Controller struct{}

func New() *Controller {
	return &Controller{}
}

type userQuery struct {
	Tab string `thx:"tab"`
}

func (c *Controller) Routes() thx.Routes {
	return thx.WithLayout(layout,
		thx.Get("/", thxauth.Get(c.getIndex)),
		thx.Get("/users/{id}", thxauth.Get(c.getUser, auth.OwnsUser)),
		thx.SSE("/events", c.events),
		thx.WS("/ws", c.ws),

		thx.HandleNotFound(func(ctx thx.Context) templ.Component {
			return notFound()
		}),
	)
}

func (c *Controller) getIndex(ctx auth.Context, _ struct{}) thx.Result {
	return thx.Render(ctx, index())
}

func (c *Controller) getUser(_ auth.Context, _ userQuery) thx.Result {
	return thx.Status(http.StatusOK).Empty()
}

func (c *Controller) events(ctx thx.Context, _ struct{}, stream thx.EventStream) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			i++
			if err := stream.Send(fmt.Sprintf("<span>tick %d</span>", i)); err != nil {
				return
			}
		}
	}
}

func (c *Controller) ws(ctx thx.Context, _ struct{}, conn *thx.WSConn) {
	for {
		req, err := conn.ReadHTMX(ctx)
		if err != nil {
			return
		}

		text, _ := req.Values["message"].(string)

		if err := conn.WriteHTMX(ctx, thx.WSMessage{
			Target:  "#ws-output",
			Swap:    "innerHTML",
			Payload: fmt.Sprintf("echo: %s", text),
		}); err != nil {
			return
		}
	}
}
