package thx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/go-thx/thx/internal"
)

// WSHandler is a handler function for WebSocket connections.
// Q is the query parameter type.
type WSHandler[Q any] func(ctx Context, query Q, conn *WSConn)

// WS registers a WebSocket route at the given path. The handler receives
// a WSConn for bidirectional communication. Optional AcceptOptions
// configure the WebSocket upgrade; by default, origin checking is skipped.
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

// Apply registers the WebSocket route on the router's mux as a GET handler
// that upgrades the connection to WebSocket.
func (r *wsRoute[Q]) Apply(router *Router) {
	path := routePath(router.Path, r.path)

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

// WSConn wraps a WebSocket connection with convenience methods
// for text, JSON, and HTMX message exchange.
type WSConn struct {
	conn *websocket.Conn
}

// ReadText reads a single text message from the WebSocket connection.
func (c *WSConn) ReadText(ctx context.Context) (string, error) {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReadJSON reads a text message and unmarshals it as JSON into v.
func (c *WSConn) ReadJSON(ctx context.Context, v any) error {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Write sends a text message over the WebSocket connection.
func (c *WSConn) Write(ctx context.Context, msg string) error {
	return c.conn.Write(ctx, websocket.MessageText, []byte(msg))
}

// WriteJSON marshals v as JSON and sends it as a text message.
func (c *WSConn) WriteJSON(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.conn.Write(ctx, websocket.MessageText, b)
}

// WSRequest is the HTMX 4.0 WebSocket request envelope sent by the client.
type WSRequest struct {
	Type      string         `json:"type"`
	RequestID string         `json:"request_id"`
	Event     string         `json:"event"`
	Path      string         `json:"path"`
	Headers   map[string]any `json:"headers"`
	Values    map[string]any `json:"values"`
}

// ReadHTMX reads and decodes an HTMX 4.0 WebSocket request envelope.
func (c *WSConn) ReadHTMX(ctx context.Context) (WSRequest, error) {
	var req WSRequest
	if err := c.ReadJSON(ctx, &req); err != nil {
		return req, err
	}
	return req, nil
}

// WSMessage is the HTMX 4.0 WebSocket response envelope sent to the client.
type WSMessage struct {
	Channel string `json:"channel,omitempty"`
	Format  string `json:"format,omitempty"`
	Target  string `json:"target,omitempty"`
	Swap    string `json:"swap,omitempty"`
	Payload string `json:"payload"`
}

// WriteHTMX sends an HTMX 4.0 WebSocket response envelope.
func (c *WSConn) WriteHTMX(ctx context.Context, msg WSMessage) error {
	return c.WriteJSON(ctx, msg)
}

// Close gracefully closes the WebSocket connection with a normal closure status.
func (c *WSConn) Close(reason string) error {
	return c.conn.Close(websocket.StatusNormalClosure, reason)
}

// Conn returns the underlying websocket.Conn for advanced use cases.
func (c *WSConn) Conn() *websocket.Conn {
	return c.conn
}
