package thx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-thx/thx/internal"
)

// SSEHandler is a handler function for Server-Sent Events connections.
// Q is the query parameter type.
type SSEHandler[Q any] func(ctx Context, query Q, stream EventStream)

// EventStream provides methods for sending Server-Sent Events to the client.
type EventStream interface {
	// Send writes a data-only SSE message.
	Send(data string) error
	// SendEvent writes an SSE message with a named event type.
	SendEvent(event string, data string) error
	// SendJSON marshals data as JSON and sends it as an SSE data field.
	SendJSON(data any) error
}

// SSE registers a Server-Sent Events route at the given path.
// The handler receives an EventStream for pushing events to the client.
// The connection stays open until the handler returns or the client disconnects.
func SSE[Q any](path string, handler SSEHandler[Q]) Route {
	return &sseRoute[Q]{
		path:    path,
		handler: handler,
	}
}

type sseRoute[Q any] struct {
	path    string
	handler SSEHandler[Q]
}

// Apply registers the SSE route on the router's mux as a GET handler
// that sets the appropriate SSE headers and streams events.
func (r *sseRoute[Q]) Apply(router *Router) {
	path := routePath(router.Path, r.path)

	router.Mux.HandleFunc("GET "+path, func(res http.ResponseWriter, req *http.Request) {
		defer handlePanic(path, router, res, req)

		flusher, ok := res.(http.Flusher)
		if !ok {
			slog.ErrorContext(req.Context(), "ResponseWriter does not support flushing.",
				"path", path,
			)
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		queryData, ok := decodeQuery[Q](req, res, router)
		if !ok {
			return
		}

		res.Header().Set("Content-Type", "text/event-stream")
		res.Header().Set("Cache-Control", "no-cache")
		res.Header().Set("Connection", "keep-alive")
		res.WriteHeader(http.StatusOK)
		flusher.Flush()

		ctx := internal.NewContext(req, res)

		stream := &eventStream{
			res:     res,
			flusher: flusher,
		}

		r.handler(ctx, queryData, stream)
	})
}

type eventStream struct {
	res     http.ResponseWriter
	flusher http.Flusher
}

// Send writes a data-only SSE message and flushes it to the client.
func (s *eventStream) Send(data string) error {
	if _, err := fmt.Fprintf(s.res, "data: %s\n\n", data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// SendEvent writes an SSE message with a named event type and flushes it.
func (s *eventStream) SendEvent(event string, data string) error {
	if _, err := fmt.Fprintf(s.res, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// SendJSON marshals data as JSON and sends it as an SSE data field.
func (s *eventStream) SendJSON(data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal SSE data: %w", err)
	}
	return s.Send(string(b))
}
