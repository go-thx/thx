package auth

import "github.com/go-thx/thx/internal"

// Context is an authenticated request context that provides access
// to the auth entity of type T via the Auth() method.
type Context[T any] = internal.AuthContext[T]
