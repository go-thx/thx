package thx

import (
	"github.com/a-h/templ"
)

// FlashOOBRenderer turns pending flash messages into a component that is
// appended to HTMX responses as an out-of-band swap.
type FlashOOBRenderer func([]FlashMessage) templ.Component

type flashOOBKey struct{}

type flashOOBConfig struct {
	selector string
	strategy string
	render   FlashOOBRenderer
}

// WithFlashOOB makes pending flash messages render automatically on every
// HTML response of the given routes, without calling SwapOOB by hand.
// The selector and strategy are passed through to OOBWithStrategy, and the
// renderer produces the markup — thx dictates no container or toast markup.
//
//	thx.WithFlashOOB("#flashes", thx.SwapAfterBegin, toasts,
//	    thx.Get("/dashboard", dashboard),
//	)
//
// Only HTMX requests are touched (history restores excluded, since HTMX
// ignores OOB swaps there). On those requests the OOB swap owns the flashes:
// they are consumed before the primary component renders, so a layout calling
// Flashes gets nil. Responses that are not rendered HTML — redirects, JSON,
// raw — leave the flash cookie intact for the next page load.
func WithFlashOOB(selector string, strategy string, render FlashOOBRenderer, routes ...Route) Routes {
	cfg := &flashOOBConfig{selector: selector, strategy: strategy, render: render}

	return Routes{wrapper(func(r *Router) {
		inner := &Router{
			Mux:             r.Mux,
			path:            r.path,
			layouts:         r.layouts,
			flashOOB:        cfg,
			errorHandler:    r.errorHandler,
			notFoundHandler: r.notFoundHandler,
			middleware:      r.middleware,
		}

		for _, route := range routes {
			route.Apply(inner)
		}

		r.errorHandler = inner.errorHandler
		if inner.notFoundHandler != nil {
			r.notFoundHandler = inner.notFoundHandler
		}
		r.middleware = inner.middleware
	})}
}

// consumeFlashOOB claims the pending flash messages for an automatic OOB
// swap. It must be called before the response body is written, as it deletes
// the flash cookie. The second return value reports whether a swap was built.
func consumeFlashOOB(ctx Context) (OOBSwap, bool) {
	cfg, ok := ctx.Value(flashOOBKey{}).(*flashOOBConfig)
	if !ok || cfg == nil || cfg.render == nil {
		return OOBSwap{}, false
	}

	// Only partial requests: a boosted or history-restore response replaces
	// the whole body, wiping the element the OOB swap just wrote into. Those
	// render with layouts, so the layout's own Flashes call shows them.
	hx := ctx.HTMX()
	if !hx.IsRequest() || !hx.IsPartial() || hx.IsHistoryRestoreRequest() {
		return OOBSwap{}, false
	}

	if consumed, _ := ctx.Value(flashConsumedKey{}).(bool); consumed {
		return OOBSwap{}, false
	}

	// Unlike Flashes, this also picks up messages queued by the handler
	// itself — for an HTMX request they are shown now, not on the next load.
	flashes := getPending(ctx)
	if len(flashes) == 0 {
		return OOBSwap{}, false
	}

	// When the handler called Flash on this request, its cookie write already
	// sits in the response and the delete below lands after it — the browser
	// applies both in order, so the cookie ends up gone. Deferring the write
	// instead would need Redirect to stop flushing headers from inside the
	// handler, which is not worth one redundant Set-Cookie header.
	ctx.SetValue(flashConsumedKey{}, true)
	ctx.SetValue(flashPendingKey{}, []FlashMessage(nil))
	clearFlashCookie(ctx)

	return OOBWithStrategy(cfg.selector, cfg.strategy, cfg.render(flashes)), true
}
