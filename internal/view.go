package internal

import (
	"context"
	"net/http"
)

type View interface {
	Out(ctx context.Context, w http.ResponseWriter) error
}

func Empty() View {
	return &emptyView{}
}

type emptyView struct{}

func (v *emptyView) Out(_ context.Context, _ http.ResponseWriter) error {
	return nil
}
