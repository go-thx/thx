package thx

import "github.com/go-thx/thx/internal"

// Context is the request-scoped context passed to all handlers.
// It extends context.Context with HTTP helpers for cookies, headers,
// path params, HTMX integration, redirects, and layout control.
type Context = internal.Context

// LocationOptions configures a client-side HTMX navigation via HX-Location.
type LocationOptions = internal.LocationOptions
