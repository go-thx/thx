package mw

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
)

type csrfTokenKey struct{}

const (
	csrfCookieName = "__csrf"
	csrfFieldName  = "_csrf"
	csrfHeaderName = "X-CSRF-Token"
)

// CSRF protects against cross-site request forgery. It generates a per-session
// secret stored in an HttpOnly cookie and derives a request token from it.
// State-changing methods (POST, PUT, PATCH, DELETE) must include the token
// as a form field (_csrf) or header (X-CSRF-Token).
func CSRF(key []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secret := getOrCreateSecret(w, r)
			token := computeToken(key, secret)

			ctx := context.WithValue(r.Context(), csrfTokenKey{}, token)
			r = r.WithContext(ctx)

			if requiresValidation(r.Method) {
				submitted := r.Header.Get(csrfHeaderName)
				if submitted == "" {
					if err := r.ParseForm(); err == nil {
						submitted = r.FormValue(csrfFieldName)
					}
				}

				if !hmac.Equal([]byte(submitted), []byte(token)) {
					http.Error(w, "CSRF token mismatch", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetCSRFToken returns the CSRF token from the context, or "" if absent.
// Use this to embed the token in forms or set it as a meta tag.
func GetCSRFToken(ctx context.Context) string {
	if t, ok := ctx.Value(csrfTokenKey{}).(string); ok {
		return t
	}
	return ""
}

// CSRFFieldName returns the expected form field name for the CSRF token.
func CSRFFieldName() string {
	return csrfFieldName
}

// CSRFHeaderName returns the expected header name for the CSRF token.
func CSRFHeaderName() string {
	return csrfHeaderName
}

func getOrCreateSecret(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookieName); err == nil && c.Value != "" {
		return c.Value
	}

	b := make([]byte, 32)
	_, _ = rand.Read(b)
	secret := base64.RawURLEncoding.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    secret,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})

	return secret
}

func computeToken(key []byte, secret string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func requiresValidation(method string) bool {
	return method == http.MethodPost ||
		method == http.MethodPut ||
		method == http.MethodPatch ||
		method == http.MethodDelete
}
