package private

import (
	"fmt"
	"time"

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
			thx.SSE("/events", c.events),
			thx.WS("/ws", c.ws),
		},
	)
}

func (c *Controller) index(ctx auth.Context, _ struct{}) thx.View {
	return thx.Render(ctx, index())
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
			if err := stream.Send("time", fmt.Sprintf("tick %d", i)); err != nil {
				return
			}
		}
	}
}

func (c *Controller) ws(ctx thx.Context, _ struct{}, conn *thx.WSConn) {
	for {
		var msg map[string]any
		if err := conn.ReadJSON(ctx, &msg); err != nil {
			return
		}

		text, _ := msg["message"].(string)
		html := fmt.Sprintf(`<div id="ws-output">echo: %s</div>`, text)
		if err := conn.Write(ctx, html); err != nil {
			return
		}
	}
}
