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

// Apply registers the static file handler on the router's mux.
func (s *staticRoute) Apply(router *Router) {
	handler := http.FileServerFS(noDirectoryListing(s.fsys))

	prefix := path.Join(router.Path, s.prefix)
	pattern := prefix
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}

	router.Mux.Handle("GET "+pattern, http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer handlePanic(pattern, router, res, req)

		versioned := req.URL.Query().Has("v")
		w := &staticResponseWriter{ResponseWriter: res, versioned: versioned}

		http.StripPrefix(prefix, handler).ServeHTTP(w, req)
	}))
}

// staticResponseWriter injects cache headers based on whether
// the request has a version query parameter.
type staticResponseWriter struct {
	http.ResponseWriter
	versioned   bool
	wroteHeader bool
}

// WriteHeader sets cache headers on the first call, then delegates.
func (w *staticResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if code == http.StatusOK || code == http.StatusPartialContent {
			if w.versioned {
				w.ResponseWriter.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.ResponseWriter.Header().Set("Cache-Control", "no-cache")
			}
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write ensures headers are written before the first body write.
func (w *staticResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

type noDirFS struct {
	fs.FS
}

// noDirectoryListing wraps an fs.FS to prevent directory listing.
// Directories are only served if they contain an index.html file.
func noDirectoryListing(fsys fs.FS) fs.FS {
	return noDirFS{fsys}
}

// Open opens the named file, returning fs.ErrNotExist for directories
// that lack an index.html file.
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
