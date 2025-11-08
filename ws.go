package thx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/coder/websocket"
	"github.com/go-thx/thx/internal"
)

type WSHandler[Q any] func(ctx Context, query Q, conn *WSConn)

func WS[Q any](path string, handler WSHandler[Q], acceptOpts ...*websocket.AcceptOptions) Route {
	var opts *websocket.AcceptOptions
	if len(acceptOpts) > 0 {
		opts = acceptOpts[0]
	} else {
		opts = &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		}
	}
	return &wsRoute[Q]{
		path:          path,
		handler:       handler,
		acceptOptions: opts,
	}
}

type wsRoute[Q any] struct {
	path          string
	handler       WSHandler[Q]
	acceptOptions *websocket.AcceptOptions
}

func (r *wsRoute[Q]) Apply(router *Router) {
	path := filepath.Join(router.Path, r.path)

	router.Mux.HandleFunc("GET "+path, func(res http.ResponseWriter, req *http.Request) {
		defer handlePanic(path, router, res, req)

		queryData, ok := decodeQuery[Q](req, res, router)
		if !ok {
			return
		}

		conn, err := websocket.Accept(res, req, r.acceptOptions)
		if err != nil {
			slog.ErrorContext(req.Context(), "Failed to accept WebSocket connection.",
				"path", path,
				"error", err,
			)
			return
		}
		defer conn.CloseNow()

		ctx := internal.NewContext(req, res)

		wsConn := &WSConn{conn: conn}

		r.handler(ctx, queryData, wsConn)

		conn.Close(websocket.StatusNormalClosure, "")
	})
}

type WSConn struct {
	conn *websocket.Conn
}

func (c *WSConn) ReadText(ctx context.Context) (string, error) {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *WSConn) ReadJSON(ctx context.Context, v any) error {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (c *WSConn) Write(ctx context.Context, msg string) error {
	return c.conn.Write(ctx, websocket.MessageText, []byte(msg))
}

func (c *WSConn) WriteJSON(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.conn.Write(ctx, websocket.MessageText, b)
}

func (c *WSConn) Close(reason string) error {
	return c.conn.Close(websocket.StatusNormalClosure, reason)
}

func (c *WSConn) Conn() *websocket.Conn {
	return c.conn
}
