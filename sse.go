package thx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime/debug"

	"github.com/go-thx/thx/internal"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/schema"
	"github.com/pkg/errors"
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

		flusher, ok := res.(http.Flusher)
		if !ok {
			slog.ErrorContext(req.Context(), "ResponseWriter does not support flushing.",
				"path", path,
			)
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

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
