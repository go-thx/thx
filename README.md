<br>

<div align="center">
    <!-- https://danmarshall.github.io/google-font-to-svg-path/ -->
    <!-- Font: Ysabeau, Color: #4a4a4a, Text: thx! -->
    <img src=".github/thx.svg" alt="thx" width="200"/><br>
    <!-- Font: Inter, Color: #707070, Text: /θæŋks/ -->
    <img src=".github/thanks.svg" alt="thx" width="200"/>
    <h3>optimizing the handling of templ and htmx within go applications</h3>
</div>

<hr />

<p align="center">
    <a href="https://go.dev/doc/devel/release"><img src="https://img.shields.io/badge/go-v1.25-informational" alt="Go v1.25"></a>
    &nbsp;
    <a href="https://github.com/a-h/templ"><img src="https://img.shields.io/badge/go--templ-v0.3.1001-informational" alt="Go-Templ v0.3.1001"></a>
    &nbsp;
    <a href="https://github.com/bigskysoftware/htmx"><img src="https://img.shields.io/badge/htmx-v4.0.0--beta6-informational" alt="HTMX v4.0.0-beta6"></a>
    &nbsp;
</p>

## About

This library is a companion to the already awesome libraries go-templ and htmx.
It is the glue between those layers, making writing typesafe SSR-based web controllers a joy.

## Why thx?

You can wire up templ handlers on `http.ServeMux` yourself — it works. But you'll end up rebuilding the same glue in every project: checking `HX-Request` headers to decide between full pages and partials, manually stripping layouts, decoding form data, composing middleware, wiring auth guards, setting `Vary` headers for caching correctness, handling redirects differently for HTMX vs regular requests.

thx handles all of that so your handlers stay focused on business logic:

- **Type-safe routing** — generic `[Q, I]` parameters decode query strings, form data, and JSON bodies automatically
- **Automatic partial rendering** — HTMX requests skip layouts, boosted requests get full pages, `Vary` headers are set correctly
- **Layout composition** — nest layouts declaratively with `WithLayout`, no manual wrapping
- **HTMX integration** — full request/response header coverage via `ctx.HTMX()` with fluent swap and trigger builders
- **Auth guards** — `auth.WithGuard` protects route groups with middleware, handles redirects
- **OOB swaps** — update multiple page sections in a single response with `SwapOOB`
- **SSE and WebSocket** — first-class route types with HTMX 4.0 envelope support
- **Result types** — explicit `Render`, `JSON`, `Raw`, `Status().Empty()` — no silent behavior
- **Zero external router** — built on Go's `http.ServeMux` (Go 1.24+), no framework lock-in

## Getting started

```bash
go get github.com/go-thx/thx
```

Requires Go 1.24+ and [templ](https://github.com/a-h/templ).

### Minimal example

```go
package main

import (
    "net/http"

    "github.com/go-thx/thx"
)

func main() {
    handler := thx.New(
        thx.Get("/", getIndex),
    )

    http.ListenAndServe(":8080", handler)
}

func getIndex(ctx thx.Context, _ struct{}) thx.Result {
    return thx.Render(ctx, indexPage())
}
```

```templ
templ indexPage() {
    <h1>Hello from thx!</h1>
}
```

### Layouts, forms, and HTMX

```go
func main() {
    handler := thx.New(
        thx.WithLayout(baseLayout,
            thx.Get("/", getIndex),
            thx.Post("/greet", postGreet),
        ),
    )

    http.ListenAndServe(":8080", handler)
}

type greetForm struct {
    Name string `thx:"name"`
}

func getIndex(ctx thx.Context, _ struct{}) thx.Result {
    return thx.Render(ctx, indexPage(""))
}

func postGreet(ctx thx.Context, _ struct{}, form greetForm) thx.Result {
    return thx.Render(ctx, indexPage(form.Name))
}
```

```templ
templ baseLayout(inner templ.Component) {
    <!DOCTYPE html>
    <html>
    <head>
        <script src="https://unpkg.com/htmx.org@4"></script>
    </head>
    <body>
        @inner
    </body>
    </html>
}

templ indexPage(name string) {
    <form hx-post="/greet" hx-target="this" hx-swap="outerHTML">
        <input type="text" name="name" value={ name } placeholder="Your name" />
        <button type="submit">Greet</button>
    </form>
    if name != "" {
        <p>Hello, { name }!</p>
    }
}
```

HTMX partial requests automatically skip the layout and return just the form fragment. Full page loads get the complete HTML with `<head>` and scripts. No manual branching needed.

### Protected routes

```go
handler := thx.New(
    thx.Get("/login", getLogin),
    thx.Post("/login", postLogin),

    auth.WithGuard("/dashboard", dashboardRoutes(),
        auth.RedirectUnauthorized("/login"),
        auth.RedirectWithCurrentPath("next"),
    ),
)
```

See the reference app in `internal/test/` for a complete working example with auth, layouts, SSE, and WebSocket.

## Recipes and patterns

Common interaction patterns for use with thx. These are application-level patterns, not framework features.

### Passing data from handlers to layouts

Inner components cannot propagate data outward to layouts during rendering — by the time a component's `Render()` executes, the layout's `<head>` has already been written. Instead, set metadata on the context in the handler before returning. Layouts read it during rendering.

```go
// Define context keys in your app
type titleKey struct{}
type breadcrumbsKey struct{}

// Handler sets metadata before returning
func (c *Controller) getUser(ctx thx.Context, q userQuery) thx.Result {
    user := loadUser(q.ID)
    ctx.SetValue(titleKey{}, user.Name + " - Profile")
    ctx.SetValue(breadcrumbsKey{}, []Crumb{{"Home", "/"}, {"Users", "/users"}, {user.Name, ""}})
    return thx.Render(ctx, userProfile(user))
}
```

```templ
// Layout reads from context
templ baseLayout(inner templ.Component) {
    <html>
    <head>
        if title, ok := thx.ViewContext(ctx).Value(titleKey{}).(string); ok {
            <title>{ title }</title>
        }
    </head>
    <body>
        @inner
    </body>
    </html>
}
```

This works because the handler runs first, sets context values, then returns a `Result`. When the layout renders, it reads those values. The inner component renders last.

For HTMX partial requests, layouts are skipped entirely, so this is purely a full-page concern — partials don't need it.

### Form and query binding

The `I` and `Q` type parameters are filled by thx's built-in decoder. Fields are matched by their `thx` tag (falling back to the Go field name), and `thx:"-"` skips a field. Keys are dotted paths, so repeatable form rows and dynamic key sets bind directly:

```go
type orderForm struct {
    Customer struct {
        Name string `thx:"name"`
    } `thx:"customer"`

    Items []struct {
        SKU string `thx:"sku"`
        Qty int    `thx:"qty"`
    } `thx:"items"`

    Tags  []string          `thx:"tags"`
    Attrs map[string]string `thx:"attrs"`
}
```

```html
<input name="customer.name" value="Marc">
<input name="items.0.sku" value="A-1"><input name="items.0.qty" value="2">
<input name="items.1.sku" value="B-7"><input name="items.1.qty" value="1">
<input name="tags" value="rush"><input name="tags" value="gift">
<input name="attrs.color" value="red">
```

Indices may arrive in any order and may be sparse — missing elements stay zero. Slices also accept repeated keys without an index (`tags` twice above), scalars take the last value. Supported leaf types are strings, bools, ints, uints, floats, `[]byte`, pointers to those, and anything implementing `encoding.TextUnmarshaler`.

Two browser realities are handled for you: a checked checkbox submits `on`, which decodes to `true`, and an untouched number, date, or select input submits an empty string, which leaves the field at its zero value instead of failing. For a `*string` the empty string is kept as a value, since blank and absent differ there.

Unknown keys are ignored in query strings but rejected in form bodies, and indices are capped (1000) along with key depth (32) so a client cannot force a large allocation.

Decode failures are a `*thx.DecodeError` with a `Kind` — `thx.ErrUnknownKey`, `thx.ErrMalformedValue`, `thx.ErrLimitExceeded`, or `thx.ErrInvalidTarget` — and the offending `Key`. The kind separates a bad request from a bad handler: the first three answer `400`, while `ErrInvalidTarget` (the `I` or `Q` struct itself cannot be decoded into) answers `500`, since no request can fix it. Every failure is logged with its kind and key before the error page renders, and both `errors.Is(err, thx.ErrMalformedValue)` and `errors.As(err, &decErr)` work on it.

Decoding only fills the struct — validation stays in the handler, see the next section.

### Form validation with field-level errors

thx decodes form data into your `I` struct automatically but leaves validation to the handler. This gives you full control over how errors are presented — especially important for HTMX form submissions where you want to re-render the form with inline errors.

Use any validation library you prefer. Here's an example with `go-playground/validator`:

```go
import "github.com/go-playground/validator/v10"

var validate = validator.New(validator.WithRequiredStructEnabled())

type registerForm struct {
    Email    string `thx:"email"    validate:"required,email"`
    Password string `thx:"password" validate:"required,min=8"`
}
```

In the handler, validate and re-render with errors:

```go
func (c *Controller) postRegister(ctx thx.Context, _ struct{}, form registerForm) thx.Result {
    errs := validateStruct(form)
    if errs != nil {
        return thx.Render(ctx, registerView(form, errs))
    }

    // ... create user, set cookie, etc.
    return ctx.Redirect("/")
}
```

A small helper converts validator errors into a map your templates can use:

```go
type FieldErrors map[string]string

func (e FieldErrors) Has(field string) bool    { return e[field] != "" }
func (e FieldErrors) Get(field string) string  { return e[field] }

func validateStruct(v any) FieldErrors {
    err := validate.Struct(v)
    if err == nil {
        return nil
    }
    errs := make(FieldErrors)
    for _, fe := range err.(validator.ValidationErrors) {
        errs[fe.Field()] = fe.Error()
    }
    return errs
}
```

In your templ template, use the errors to highlight fields:

```templ
templ registerView(form registerForm, errs FieldErrors) {
    <form method="post">
        <input type="email" name="email" value={ form.Email }
               class={ templ.KV("input-error", errs.Has("Email")) } />
        if errs.Has("Email") {
            <span class="error">{ errs.Get("Email") }</span>
        }

        <input type="password" name="password"
               class={ templ.KV("input-error", errs.Has("Password")) } />
        if errs.Has("Password") {
            <span class="error">{ errs.Get("Password") }</span>
        }

        <button type="submit">Register</button>
    </form>
}
```

This pattern works with both full-page submissions and HTMX partials. The handler always runs, so you keep the submitted form values and can re-render with errors inline. No framework magic — just your validation library, your error format, your templates.

### Active search (debounced)

A search input that queries the server on keyup with a 500ms debounce:

```html
<input type="search" name="q"
    hx-get="/search"
    hx-trigger="keyup changed delay:500ms, search"
    hx-target="#results"
    hx-swap="innerHTML"
    hx-indicator="#spinner" />
<div id="results"></div>
```

The handler receives the query via thx's typed `Q` parameter and returns a partial result list.

### Infinite scroll

Trigger loading the next page when the last row becomes visible:

```html
<tr hx-get="/items?page=3"
    hx-trigger="intersect once"
    hx-swap="afterend"
    hx-indicator="#loading">
    <!-- last visible row -->
</tr>
```

The handler returns more `<tr>` elements. The last row in each batch carries the next `hx-get` to continue the chain.

### Lazy load

Load expensive content after the page renders:

```html
<div hx-get="/dashboard/stats"
     hx-trigger="load"
     hx-target="this"
     hx-swap="outerHTML">
    <!-- placeholder / skeleton -->
</div>
```

The handler returns the full component which replaces the placeholder.

### Pagination

Each page button swaps the entire table/list container:

```html
<button hx-get="/users?page=2"
        hx-target="#user-list"
        hx-swap="outerHTML">
    2
</button>
```

Use thx's typed query params to decode the page number. The response includes the list and updated pagination controls.

### Caching

thx offers three caching primitives, all with a TTL. Store the returned closure in a package-level variable so the cache persists across requests.

**`Cached`** and **`CachedByKey`** memoize a `templ.Component`. The component is built once (per key) and re-rendered with the live request context on every request, so `ViewContext` and layouts still work. Use them when the expensive part is *building* the component from already-available data:

```go
var sidebar = thx.Cached(5*time.Minute, func() templ.Component {
    return sidebarView(loadNavItems())
})

var userCard = thx.CachedByKey[string](time.Minute, func(userID string) templ.Component {
    return userCardView(loadUser(userID))
})

func (c *Controller) getPage(ctx thx.Context, _ struct{}) thx.Result {
    return thx.Render(ctx, sidebar())        // wrapped in layouts as usual
}
```

**`CachedPartial`** caches the *rendered bytes* of a partial, keyed by a request-derived key (e.g. locale). Unlike the two above, its factory receives the request `context.Context` — so it can load data with the live context on a miss — and may return an `error`, which is surfaced to the caller (HTTP 500) and never cached. Because it caches output rather than a component, consumers that bind their locale at view-context construction stay correct as long as the key includes the locale. Reach for it when the miss-time work is request-scoped or can fail (e.g. a live external API call):

```go
var greeting = thx.CachedPartial(time.Minute,
    func(ctx context.Context) string { return view.Lang(ctx) },   // cache key
    func(ctx context.Context) (templ.Component, error) {
        data, err := api.FetchGreeting(ctx)
        if err != nil {
            return nil, err
        }
        return greetingView(data), nil
    },
    thx.WithCacheControl(),                                        // optional
)

func (c *Controller) getGreeting(ctx thx.Context, _ struct{}) thx.Result {
    return greeting(ctx)                                          // returns a Result directly
}
```

`CachedPartial` renders without layouts (like `Partial`). On a miss or once an entry is stale it re-renders synchronously, single-flighted so a burst of callers triggers one render they all share. If a re-render fails while stale bytes are still held, the stale bytes are served (availability over freshness). `WithCacheControl()` adds `Cache-Control: private, max-age=<ttl>` so clients cache the fragment for the same duration.

### Flash messages and toast notifications

There are three patterns depending on the context. Use the right one for each situation.

#### HTMX requests: HX-Trigger (recommended)

The simplest approach for HTMX form submissions. The server triggers a client-side event, and a small JS listener renders the toast. This works alongside a normal content swap — a single response can update the page AND show a notification.

```go
func (c *Controller) postSomething(ctx thx.Context, _ struct{}, form myForm) thx.Result {
    // ... do work ...
    ctx.HTMX().Trigger("showToast", map[string]any{"level": "success", "message": "Saved!"})
    return thx.Render(ctx, updatedContent())
}
```

```javascript
// ~5 lines in your base layout
document.body.addEventListener("showToast", (e) => {
    const { level, message } = e.detail;
    // render your toast UI however you like
});
```

For error cases where you don't want to swap content, combine with `Reswap`:

```go
ctx.HTMX().Reswap(thx.SwapNone)
ctx.HTMX().Trigger("showToast", map[string]any{"level": "error", "message": "Validation failed"})
return thx.Empty()
```

#### HTMX requests: OOB swap (fully server-rendered)

If you want toasts rendered entirely server-side (no client JS for toast construction), use OOB swaps. Your layout needs a `<div id="flashes"></div>` container, and the server appends toast HTML as an OOB fragment.

```go
func (c *Controller) postSomething(ctx thx.Context, _ struct{}, form myForm) thx.Result {
    // ... do work ...
    return thx.SwapOOB(ctx, updatedContent(),
        thx.OOBWithStrategy("#flashes", thx.SwapAfterBegin, toastSuccess("Saved!")),
    )
}
```

Using `afterbegin` appends new toasts without clearing existing ones.

#### HTMX requests: automatic flash OOB

If every handler ends up repeating the same OOB swap, opt into `WithFlashOOB`. It appends pending flash messages to every rendered HTML response of the wrapped routes, so handlers only call `thx.Flash*`:

```go
thx.New(
    thx.WithFlashOOB("#flashes", thx.SwapAfterBegin, toasts,
        thx.Get("/dashboard", c.getDashboard),
        thx.Post("/settings", c.postSettings),
    ),
)

// toasts is yours — thx never dictates the container or the markup
templ toasts(flashes []thx.FlashMessage) {
    for _, f := range flashes {
        @toast(f.Level, f.Message)
    }
}
```

```go
func (c *Controller) postSettings(ctx thx.Context, _ struct{}, form settingsForm) thx.Result {
    // ... do work ...
    thx.FlashSuccess(ctx, "Saved!")
    return thx.Render(ctx, updatedContent())
}
```

Only HTMX requests are touched — history restores excluded, since HTMX ignores OOB swaps there. On those requests the OOB swap owns the flashes: they are consumed before the primary component renders, so a layout calling `Flashes` gets nil. Responses that are not rendered HTML — redirects, `JSON`, `Raw`, `Empty` — leave the cookie intact, so a flash set before `ctx.Redirect` still shows on the next page load. The swap is appended after any explicit `SwapOOB` swaps.

#### Full-page redirects: cookie-based flash

For redirects that leave the HTMX context (login, logout, initial navigation), use thx's cookie-based flash messages. These survive across the redirect and are consumed on the next page load.

```go
// POST handler — set flash before redirect
func (c *Controller) postLogin(ctx thx.Context, _ struct{}, form loginForm) thx.Result {
    // ... authenticate ...
    thx.FlashSuccess(ctx, "Welcome back!")
    return ctx.Redirect("/dashboard")
}

// GET handler — read and display flashes
func (c *Controller) getDashboard(ctx thx.Context, _ struct{}) thx.Result {
    flashes := thx.Flashes(ctx) // reads and clears cookie
    return thx.Render(ctx, dashboardView(flashes))
}
```

Note: `HX-Trigger` headers on 3xx responses are ignored by HTMX, so cookie-based flashes are the only option for redirects.

### Out-of-band updates

Update multiple page sections in a single response using thx's OOB helpers:

```go
func (c *Controller) postComment(ctx thx.Context, _ struct{}, form commentForm) thx.Result {
    // ... save comment ...
    return thx.SwapOOB(ctx, commentList(comments),
        thx.OOBWithStrategy("#comment-count", thx.SwapInnerHTML, commentCount(len(comments))),
    )
}
```

The primary component renders normally with layouts. OOB swaps are appended for HTMX requests only.

## Intentionally not included

### CSRF protection

thx does not include CSRF token middleware. For a typical thx application, browser-level protections are sufficient:

- **`SameSite=Lax` cookies** — thx sets this on all cookies by default. The browser will not send auth cookies on cross-origin POST/PUT/DELETE form submissions, which prevents classic CSRF attacks.
- **HTMX custom headers** — HTMX adds `HX-Request: true` on every request. Custom headers trigger CORS preflight, so cross-origin HTMX requests are blocked unless the server explicitly allows them.

Together, these cover the standard CSRF attack surface without tokens. If your application has unusual requirements (complex subdomain trust models, very old browser support), use a dedicated CSRF middleware like `gorilla/csrf` or `justinas/nosurf` via `thx.WithMiddleware`.

### Request ID

Request ID generation (for logging/tracing correlation) is a standard HTTP middleware — it generates a UUID, stores it in context, and sets a response header. There is no framework integration needed. Use any existing middleware or write your own:

```go
func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := uuid.NewString()
        ctx := context.WithValue(r.Context(), requestIDKey{}, id)
        w.Header().Set("X-Request-ID", id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

Wire it in with `thx.WithMiddleware(RequestID, ...)`.

### Flash conventions by default

Auto-appended flash OOB swaps are opt-in via `WithFlashOOB`, never on by default. The container selector, the swap strategy, and the markup all come from the application — thx supplies no toast component, no `#flashes` convention, and no client JS. Toast presentation varies wildly between apps (libraries, animations, positioning, stacking), so the framework only handles the plumbing: consume pending flashes, render the app's component, append it as an OOB template.

Without the opt-in, the building blocks stand alone: `ctx.HTMX().Trigger()` for HTMX requests (~5 lines of client JS), `thx.SwapOOB()` for fully server-rendered toasts, and `thx.Flash()` for redirects.

See [Flash messages and toast notifications](#flash-messages-and-toast-notifications) in the recipes section for all patterns.

## References and inspiration

### [go-htmx](https://github.com/donseba/go-htmx)

A per-request HTMX helper layer for Go. Wraps `http.ResponseWriter` to provide typed access to all HTMX request/response headers, plus fluent builders for swap strategies and trigger events. Framework-agnostic with zero dependencies. Includes a `html/template`-based component system, SSE broadcast manager, and toast notification shortcuts via `HX-Trigger` (baked-in `showMessage` event with level/message payload). **thx** adopts the idea of typed swap and trigger builders but intentionally leaves notification conventions to the application. Otherwise covers the same ground more deeply — automatic partial rendering, `Vary` headers, type-safe generic handlers, route composition, and templ integration are all built-in rather than left to the developer.

### [htmx-go](https://github.com/angelofallars/htmx-go)

A stateless, functional Go package for HTMX header handling with zero dependencies. Provides package-level functions for reading request headers and a `Response` builder (value type, chainable) for setting response headers. Features a `SwapStrategy` typed string with modifier methods that deduplicate by prefix, three trigger types (`Trigger`, `TriggerDetail`, `TriggerObject`) with automatic JSON serialization, and templ integration via duck-typed interface (no import dependency). Notable for `PreventPushURL`/`PreventReplaceURL` and `scroll:window`/`show:window` modifiers. **thx** adopted swap modifier deduplication, window scroll/show targets, prevent push/replace URL, and native `time.Duration` formatting from this project's approach. thx goes further with automatic partial rendering, `Vary` headers, context-integrated HTMX API, layout composition, and SSE/WebSocket support.

### [htmgo](https://github.com/maddalax/htmgo)

A full Go+HTMX framework that replaces templating with a pure Go HTML DSL — every element is a function call (`h.Div(h.Class("..."), h.Text("..."))`) with attributes and children as variadic arguments. Built on chi router with convention-based routing via AST code generation (pages in `pages/`, partials in `partials/`). Features type-safe partial references via reflection, a rich JS command system for common interactions without writing JavaScript (`OnClick(h.ToggleClass("hidden"))`), component-level caching with TTL and per-key variants, OOB swap helpers for multi-target updates, and query param fallback to `HX-Current-URL` for partial handlers. **thx** adopted the ideas of component caching, OOB swap helpers, and query param fallback from this project. thx takes a different path overall — templ for HTML, generic type parameters for type-safe form/query binding, automatic layout composition and partial rendering, and stdlib-only routing without code generation.

### [goshipit](https://github.com/haatos/goshipit)

A templ component library wrapping DaisyUI 5, distributed via a CLI that copies `.templ` source files into your project (shadcn/ui model). Not a framework — no routing, no middleware. Provides ~45 pre-built components with consistent props-struct APIs. The HTMX-integrated components (active search, infinite scroll, pagination, date picker, lazy load) demonstrate the real patterns every Go+HTMX app needs, though with hardcoded URLs and implicit handler contracts. Notable for a server-driven toast pattern using `HX-Retarget: body` + `HX-Reswap: beforeend` to inject notifications from any handler without client-side JS listeners. **thx** does not ship UI components — that is application territory — but documents the HTMX interaction patterns as recipes (see above). thx's type-safe routing would make these patterns compile-time safe rather than relying on string URLs.

### [Go Axum Handlers](https://kubuzetto.github.io/posts/go-axum-handlers/) ([+Part 2](https://kubuzetto.github.io/posts/go-axum-handlers-pt2/))

Explores bringing Rust's axum pattern to Go: handler functions declare their needs through parameter types (each implementing an `Extractor` interface), and the framework automatically parses requests and serializes responses. Part 2 optimizes this by packing extractors into a struct and using `unsafe.Pointer` field offsets to avoid per-request reflection.

The pattern is powerful for general-purpose REST APIs with many diverse input sources, but requires reflection or unsafe code to inspect handler signatures at runtime. **thx** takes a different approach: fixed generic type parameters `[Q, I, O]` (query, input, output) give fully type-safe extraction at compile time with zero reflection — a simpler trade-off that fits the narrow templ+htmx domain well.

### [gorilla/schema](https://github.com/gorilla/schema)

The de-facto standard for filling Go structs from `url.Values`. Matches keys via a configurable alias tag, descends into nested structs and slices with dotted and indexed keys (`user.addr.city`, `items.0.name`), supports `map` fields, per-type converters, `IgnoreUnknownKeys`, `ZeroEmpty` for blank inputs, and aggregates every failure into a `MultiError` keyed by field path instead of stopping at the first one. **thx** has a built-in decoder in the same spirit — the `thx` struct tag, dotted keys for nested structs, registered converters, `encoding.TextUnmarshaler` support, and a type-keyed field cache warmed at route registration so no per-request type walk happens. Dotted and indexed keys, `map` fields, and blank-input-means-zero (gorilla's `ZeroEmpty`) are all present. It deliberately stays smaller: no dependency, and one error at a time rather than a `MultiError` — validation and field-level error reporting are the handler's job in thx, so aggregating decode errors would duplicate it. Unknown keys are fatal for form bodies but ignored for query strings, which matches how the two are actually used.

### [ajg/form](https://github.com/ajg/form)

An encoder *and* decoder for `application/x-www-form-urlencoded`, shaped like `encoding/json`. Flattens arbitrarily nested maps, slices, arrays, and structs into dot-delimited paths, with the delimiter and escape character configurable (`DelimitWith`, `EscapeWith`) so literal dots in keys round-trip. Handles `time.Time`, `url.URL`, `math/big`, and `image/color` as leaf values, falls back to `json` tags, supports `omitempty`, `IgnoreCase`, and a `KeysWith` hook to rewrite field names (e.g. to snake_case). Notable for treating untrusted input as untrusted: `MaxSize`, `MaxDepth`, and `MaxBytes` bound allocation, and failures come back as a typed `*form.Error` classifying parse errors, unknown keys, and limit violations. **thx** takes the decode-only half — it never needs to encode, since templ renders the HTML. It adopted four ideas from this project: indexed keys for repeatable form rows (`items.0.name`), `map` fields for dynamic key sets, bounded indices and key depth so those two cannot be turned into an allocation attack, and errors classified by kind so a malformed request can be told apart from an undecodable target struct. The encoder, complex numbers, colors, `math/big`, and configurable delimiters are outside what an htmx form submission needs.
