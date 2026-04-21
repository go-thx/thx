package mw

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
)

type nonceKey struct{}

// Nonce generates a cryptographic nonce per request, stores it in the
// context, and sets a Content-Security-Policy header that allows scripts
// and styles only with the matching nonce.
func Nonce(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		nonce := base64.StdEncoding.EncodeToString(b)

		ctx := context.WithValue(r.Context(), nonceKey{}, nonce)

		csp := fmt.Sprintf(
			"default-src 'self'; script-src 'nonce-%s'; style-src 'nonce-%s'; img-src 'self'; connect-src 'self' ws: wss:",
			nonce, nonce,
		)
		w.Header().Set("Content-Security-Policy", csp)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetNonce returns the CSP nonce from the context, or "" if absent.
func GetNonce(ctx context.Context) string {
	if n, ok := ctx.Value(nonceKey{}).(string); ok {
		return n
	}
	return ""
}
