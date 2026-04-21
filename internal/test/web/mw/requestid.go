package mw

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type requestIDKey struct{}

// RequestID generates a unique request ID for each request, stores it in
// the context, and sets it as the X-Request-ID response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := generateID(16)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the request ID from the context, or "" if absent.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

func generateID(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
