package thx

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/a-h/templ"
)

// Cached wraps a templ component factory with time-based caching.
// The component is rendered once and the HTML output is reused for
// the given duration. After expiry, the next render re-invokes the
// factory to produce fresh output.
//
//	var sidebar = thx.Cached(5*time.Minute, func() templ.Component {
//	    return sidebarTemplate(loadNavItems())
//	})
//
// Use in a handler: thx.Render(ctx, sidebar())
func Cached(ttl time.Duration, factory func() templ.Component) func() templ.Component {
	var (
		mu      sync.RWMutex
		html    []byte
		expires time.Time
	)

	return func() templ.Component {
		mu.RLock()
		if time.Now().Before(expires) {
			cached := html
			mu.RUnlock()
			return rawComponent(cached)
		}
		mu.RUnlock()

		mu.Lock()
		defer mu.Unlock()

		// Double-check after acquiring write lock.
		if time.Now().Before(expires) {
			return rawComponent(html)
		}

		var buf bytes.Buffer
		comp := factory()
		if err := comp.Render(context.Background(), &buf); err != nil {
			return comp
		}

		html = buf.Bytes()
		expires = time.Now().Add(ttl)
		return rawComponent(html)
	}
}

// CachedByKey wraps a keyed templ component factory with per-key caching.
// Each unique key gets its own cached HTML output with the given TTL.
//
//	var userCard = thx.CachedByKey[string](time.Minute, func(userID string) templ.Component {
//	    return userCardTemplate(loadUser(userID))
//	})
//
// Use in a handler: thx.Render(ctx, userCard("user-123"))
func CachedByKey[K comparable](ttl time.Duration, factory func(K) templ.Component) func(K) templ.Component {
	var mu sync.RWMutex
	entries := make(map[K]*cacheEntry)

	return func(key K) templ.Component {
		mu.RLock()
		if e, ok := entries[key]; ok && time.Now().Before(e.expires) {
			cached := e.html
			mu.RUnlock()
			return rawComponent(cached)
		}
		mu.RUnlock()

		mu.Lock()
		defer mu.Unlock()

		if e, ok := entries[key]; ok && time.Now().Before(e.expires) {
			return rawComponent(e.html)
		}

		var buf bytes.Buffer
		comp := factory(key)
		if err := comp.Render(context.Background(), &buf); err != nil {
			return comp
		}

		entries[key] = &cacheEntry{
			html:    buf.Bytes(),
			expires: time.Now().Add(ttl),
		}
		return rawComponent(entries[key].html)
	}
}

type cacheEntry struct {
	html    []byte
	expires time.Time
}

type rawComponent []byte

func (r rawComponent) Render(_ context.Context, w io.Writer) error {
	_, err := w.Write(r)
	return err
}
