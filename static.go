package thx

import (
	"io/fs"
	"net/http"
	"strings"
)

// Static serves static files from the given filesystem at the given URL prefix.
// Files requested with a "v" query parameter get immutable cache headers
// (1 year, immutable). Files without it get standard caching (no-cache).
func Static(prefix string, fsys fs.FS) Route {
	return &staticRoute{prefix: prefix, fsys: fsys}
}

type staticRoute struct {
	prefix string
	fsys   fs.FS
}

func (s *staticRoute) Apply(router *Router) {
	handler := http.FileServerFS(s.fsys)

	pattern := s.prefix
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}

	router.Mux.Handle("GET "+pattern, http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer handlePanic(pattern, router, res, req)

		if req.URL.Query().Has("v") {
			res.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			res.Header().Set("Cache-Control", "no-cache")
		}

		http.StripPrefix(s.prefix, handler).ServeHTTP(res, req)
	}))
}
