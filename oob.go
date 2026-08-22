package thx

import (
	"bytes"
	"html"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-thx/thx/internal"
)

// OOBSwap represents an out-of-band swap targeting a specific element.
type OOBSwap struct {
	selector string
	strategy string
	comp     templ.Component
}

// OOB creates an out-of-band swap that replaces the element matching
// the component's root element ID (using hx-swap-oob="true").
func OOB(comp templ.Component) OOBSwap {
	return OOBSwap{comp: comp, strategy: "true"}
}

// OOBWithStrategy creates an out-of-band swap with a specific swap strategy
// targeting the given CSS selector.
//
//	thx.OOBWithStrategy("#notifications", thx.SwapInnerHTML, notificationList())
func OOBWithStrategy(selector string, strategy string, comp templ.Component) OOBSwap {
	return OOBSwap{selector: selector, strategy: strategy, comp: comp}
}

// SwapOOB returns a Result that renders the primary component followed by
// one or more out-of-band swaps. For non-HTMX requests, only the primary
// component is rendered.
func SwapOOB(ctx Context, primary templ.Component, swaps ...OOBSwap) Result {
	return &oobResult{ctx: ctx, primary: primary, swaps: swaps}
}

type oobResult struct {
	ctx     Context
	primary templ.Component
	swaps   []OOBSwap
	status  int
}

// WriteResult renders the primary component with layouts, then appends
// out-of-band swap templates for HTMX requests.
func (r *oobResult) WriteResult(res http.ResponseWriter) error {
	comp := r.primary
	if layouts, ok := r.ctx.Value(layoutsKey{}).([]Layout); ok {
		comp = applyLayouts(r.ctx, comp, layouts)
	}
	flashSwap, hasFlash := consumeFlashOOB(r.ctx)

	if r.status > 0 {
		res.WriteHeader(r.status)
	}

	if err := comp.Render(r.ctx, res); err != nil {
		return err
	}

	if !r.ctx.HTMX().IsRequest() {
		return nil
	}

	for _, swap := range r.swaps {
		if err := renderOOBSwap(r.ctx, res, swap); err != nil {
			return err
		}
	}

	if !hasFlash {
		return nil
	}

	return renderOOBSwap(r.ctx, res, flashSwap)
}

// renderOOBSwap renders a single OOB swap as an hx-swap-oob template element.
func renderOOBSwap(ctx internal.Context, res http.ResponseWriter, swap OOBSwap) error {
	var buf bytes.Buffer
	if err := swap.comp.Render(ctx, &buf); err != nil {
		return err
	}

	attr := html.EscapeString(swap.strategy)
	if swap.selector != "" {
		attr += ":" + html.EscapeString(swap.selector)
	}

	if _, err := res.Write([]byte(`<template hx-swap-oob="` + attr + `">`)); err != nil {
		return err
	}
	if _, err := res.Write(buf.Bytes()); err != nil {
		return err
	}
	_, err := res.Write([]byte(`</template>`))
	return err
}
