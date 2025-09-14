package thx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime/debug"

	"github.com/coder/websocket"
	"github.com/go-thx/thx/internal"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/schema"
	"github.com/pkg/errors"
)

type WSHandler[Q any] func(ctx Context, query Q, conn *WSConn)

type WSOption func(*wsOptions)

type wsOptions struct {
	acceptOptions *websocket.AcceptOptions
}

func WSAcceptOptions(opts *websocket.AcceptOptions) WSOption {
	return func(o *wsOptions) {
		o.acceptOptions = opts
	}
}

func WS[Q any](path string, handler WSHandler[Q], opts ...WSOption) Route {
	o := &wsOptions{
		acceptOptions: &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		},
	}
	for _, opt := range opts {
		opt(o)
	}
	return &wsRoute[Q]{
		path:          path,
		handler:       handler,
		acceptOptions: o.acceptOptions,
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
		defer func() {
			if reason := recover(); reason != nil {
				if err, ok := reason.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(reason)
				}

				slog.ErrorContext(req.Context(), "Recovered from panic.",
					"path", path,
					"reason", reason,
					"stack", string(debug.Stack()),
				)
			}
		}()

		decoder := schema.NewDecoder()

		var queryData Q

		if err := decoder.Decode(&queryData, req.URL.Query()); err != nil {
			slog.WarnContext(req.Context(), "Failed to decode query params.",
				"error", err,
			)
			res.WriteHeader(http.StatusBadRequest)
			if router.ErrorHandler != nil {
				ctx := internal.NewContext(req, res)
				router.ErrorHandler(ctx, res, req, err)
			}
			return
		}

		validate := validator.New(validator.WithRequiredStructEnabled())

		if err := validate.StructCtx(req.Context(), queryData); err != nil {
			slog.WarnContext(req.Context(), "Validation of query params failed.",
				"error", err,
			)
			res.WriteHeader(http.StatusBadRequest)
			if router.ErrorHandler != nil {
				ctx := internal.NewContext(req, res)
				router.ErrorHandler(ctx, res, req, err)
			}
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

func (c *WSConn) WriteText(ctx context.Context, msg string) error {
	return c.conn.Write(ctx, websocket.MessageText, []byte(msg))
}

func (c *WSConn) WriteHTML(ctx context.Context, html string) error {
	return c.conn.Write(ctx, websocket.MessageText, []byte(html))
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
