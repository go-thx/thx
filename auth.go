package thx

import (
	"context"

	"github.com/go-thx/thx/internal"
)

func SetAuth[T any](ctx context.Context, auth T) context.Context {
	return internal.SetAuth(ctx, auth)
}
