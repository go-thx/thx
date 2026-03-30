package thx

import (
	"context"

	"github.com/go-thx/thx/internal"
)

// SetAuth stores the authenticated entity in the context.
// Call this in your auth middleware after verifying the user.
// The entity can later be retrieved via auth.Context[T].Auth().
func SetAuth[T any](ctx context.Context, auth T) context.Context {
	return internal.SetAuth(ctx, auth)
}
