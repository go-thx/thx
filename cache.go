package thx

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/a-h/templ"
	"golang.org/x/sync/singleflight"
)

// Cached wraps a templ component factory with time-based caching.
// The factory is called once and the resulting component is reused for
// the given duration. After expiry, the next call re-invokes the factory.
// The component is rendered with the actual request context each time,
// so thx.ViewContext and layouts work correctly.
//
//	var sidebar = thx.Cached(5*time.Minute, func() templ.Component {
//	    return sidebarTemplate(loadNavItems())
//	})
//
// Use in a handler: thx.Render(ctx, sidebar())
func Cached(ttl time.Duration, factory func() templ.Component) func() templ.Component {
	var (
		mu      sync.RWMutex
		comp    templ.Component
		expires time.Time
	)

	return func() templ.Component {
		mu.RLock()
		if time.Now().Before(expires) {
			c := comp
			mu.RUnlock()
			return c
		}
		mu.RUnlock()

		mu.Lock()
		defer mu.Unlock()

		if time.Now().Before(expires) {
			return comp
		}

		comp = factory()
		expires = time.Now().Add(ttl)
		return comp
	}
}

// CachedByKey wraps a keyed templ component factory with per-key caching.
// Each unique key gets its own cached component with the given TTL.
// Expired entries are evicted on cache miss.
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
		now := time.Now()

		mu.RLock()
		if e, ok := entries[key]; ok && now.Before(e.expires) {
			c := e.comp
			mu.RUnlock()
			return c
		}
		mu.RUnlock()

		mu.Lock()
		defer mu.Unlock()

		now = time.Now()

		// Evict the looked-up key if expired.
		if e, ok := entries[key]; ok {
			if now.After(e.expires) {
				delete(entries, key)
			} else {
				return e.comp
			}
		}

		c := factory(key)
		entries[key] = &cacheEntry{
			comp:    c,
			expires: now.Add(ttl),
		}
		return c
	}
}

// cacheEntry holds a cached component and its expiration time.
type cacheEntry struct {
	comp    templ.Component
	expires time.Time
}

// CachedOption configures CachedRender.
type CachedOption func(*cachedConfig)

type cachedConfig struct {
	cacheControl bool
}

// WithCacheControl makes CachedRender emit a
// "Cache-Control: private, max-age=<ttl>" response header, letting the client
// cache the fragment for the same duration the server does.
func WithCacheControl() CachedOption {
	return func(c *cachedConfig) {
		c.cacheControl = true
	}
}

// CachedRender caches the rendered bytes of a partial, keyed by a
// request-derived key (e.g. locale), and refreshes them every ttl.
//
// Unlike Cached, the factory receives the request context, so it can load
// data with the live request context on a cache miss, and it may return an
// error — which is surfaced to the caller (HTTP 500) and never cached. Output
// is cached as bytes rather than as a component, so consumers that bind their
// locale at view-context construction stay correct as long as the key includes
// the locale.
//
// On a miss, concurrent callers for the same key are single-flighted: the
// factory runs once and the rendered bytes are shared with the waiters. The
// component is rendered without layouts, like Partial. Expired entries are
// evicted on access.
//
//	var greeting = thx.CachedRender(time.Minute,
//	    func(ctx context.Context) string { return view.Lang(ctx) },
//	    func(ctx context.Context) (templ.Component, error) {
//	        data, err := api.FetchGreeting(ctx)
//	        if err != nil {
//	            return nil, err
//	        }
//	        return greetingView(data), nil
//	    },
//	    thx.WithCacheControl(),
//	)
//
//	// Use in a handler:
//	func (c *Controller) getGreeting(ctx thx.Context, _ struct{}) thx.Result {
//	    return greeting(ctx)
//	}
func CachedRender(
	ttl time.Duration,
	keyFn func(context.Context) string,
	factory func(context.Context) (templ.Component, error),
	opts ...CachedOption,
) func(Context) Result {
	var cfg cachedConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	maxAge := int(ttl.Seconds())

	var (
		mu      sync.RWMutex
		entries = make(map[string]*renderedEntry)
		group   singleflight.Group
	)

	newResult := func(body []byte) Result {
		return &cachedRenderResult{body: body, cacheControl: cfg.cacheControl, maxAge: maxAge}
	}

	return func(ctx Context) Result {
		key := keyFn(ctx)

		mu.RLock()
		if e, ok := entries[key]; ok && time.Now().Before(e.expires) {
			body := e.body
			mu.RUnlock()
			return newResult(body)
		}
		mu.RUnlock()

		body, err, _ := group.Do(key, func() (any, error) {
			// A prior flight for this key may have populated the cache while
			// this call was waiting to enter the group.
			mu.RLock()
			if e, ok := entries[key]; ok && time.Now().Before(e.expires) {
				body := e.body
				mu.RUnlock()
				return body, nil
			}
			mu.RUnlock()

			comp, err := factory(ctx)
			if err != nil {
				return nil, err
			}

			var buf bytes.Buffer
			if err := comp.Render(ctx, &buf); err != nil {
				return nil, err
			}
			body := buf.Bytes()

			mu.Lock()
			entries[key] = &renderedEntry{body: body, expires: time.Now().Add(ttl)}
			mu.Unlock()

			return body, nil
		})
		if err != nil {
			return &errorResult{err: err}
		}

		return newResult(body.([]byte))
	}
}

// renderedEntry holds cached rendered bytes and their expiration time.
type renderedEntry struct {
	body    []byte
	expires time.Time
}

// cachedRenderResult writes pre-rendered bytes, optionally with a
// Cache-Control header.
type cachedRenderResult struct {
	body         []byte
	cacheControl bool
	maxAge       int
}

// WriteResult writes the cached bytes, setting Cache-Control first if enabled.
func (r *cachedRenderResult) WriteResult(res http.ResponseWriter) error {
	if r.cacheControl {
		res.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(r.maxAge))
	}
	_, err := res.Write(r.body)
	return err
}

// errorResult surfaces a handler error through the router's writeResult path,
// which responds with HTTP 500. Nothing is cached.
type errorResult struct {
	err error
}

// WriteResult returns the wrapped error without writing a body.
func (r *errorResult) WriteResult(http.ResponseWriter) error {
	return r.err
}
