package thx

import (
	"sync"
	"time"

	"github.com/a-h/templ"
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
