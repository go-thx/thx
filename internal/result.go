package internal

import "net/http"

// Result is the interface returned by all route handlers.
// Implementations write the handler's response to the http.ResponseWriter.
type Result interface {
	WriteResult(http.ResponseWriter) error
}

// EmptyResult is a Result that writes nothing to the response.
type EmptyResult struct{}

// WriteResult is a no-op for empty results.
func (e *EmptyResult) WriteResult(_ http.ResponseWriter) error {
	return nil
}
