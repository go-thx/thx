package thx

import (
	"io/fs"
	"net/http"
	"path"
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
	handler := http.FileServerFS(noDirectoryListing(s.fsys))

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

type noDirFS struct {
	fs.FS
}

func noDirectoryListing(fsys fs.FS) fs.FS {
	return noDirFS{fsys}
}

func (n noDirFS) Open(name string) (fs.File, error) {
	f, err := n.FS.Open(name)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if info.IsDir() {
		// Check for index.html; if absent, return not found.
		index, err := n.FS.Open(path.Join(name, "index.html"))
		if err != nil {
			f.Close()
			return nil, fs.ErrNotExist
		}
		index.Close()
	}

	return f, nil
}
