package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-thx/thx"
	"golang.org/x/sync/errgroup"
	"thx.test/web/pages"
)

type Server struct {
	mux http.Handler
}

func New(pages *pages.Controller) *Server {
	mux := http.NewServeMux()

	mux.Handle("/", thx.New(pages.Routes()...))

	return &Server{
		mux: mux,
	}
}

func (s *Server) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:              "localhost:8642",
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		slog.InfoContext(ctx, "Starting web server.",
			"addr", "localhost:8642",
		)

		if err := server.ListenAndServe(); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}

			return fmt.Errorf("server stopped unexpectedly: %w", err)
		}

		return nil
	})

	<-groupCtx.Done()

	slog.WarnContext(ctx, "Shutting down web server.",
		"reason", ctx.Err(),
	)

	ctxShutDown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	slog.InfoContext(ctx, "Stopping web server.",
		"addr", "localhost:8642",
	)

	//nolint:contextcheck // cannot use given ctx, as it is already done
	if err := server.Shutdown(ctxShutDown); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	return group.Wait()
}
