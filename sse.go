package thx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/go-thx/thx/internal"
)

type SSEHandler[Q any] func(ctx Context, query Q, stream EventStream)

type EventStream interface {
	Send(event string, data string) error
	SendJSON(event string, data any) error
}

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

func (r *sseRoute[Q]) Apply(router *Router) {
	path := filepath.Join(router.Path, r.path)

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

func (s *eventStream) Send(event string, data string) error {
	if _, err := fmt.Fprintf(s.res, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *eventStream) SendJSON(event string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal SSE data: %w", err)
	}
	return s.Send(event, string(b))
}
