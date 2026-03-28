package internal

import "net/http"

type Result interface {
	WriteResult(http.ResponseWriter) error
}

type EmptyResult struct{}

func (e *EmptyResult) WriteResult(_ http.ResponseWriter) error {
	return nil
}
