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
    <a href="https://github.com/bigskysoftware/htmx"><img src="https://img.shields.io/badge/htmx-v4.0.0--alpha8-informational" alt="HTMX v4.0.0-alpha8"></a>
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
    Name string `schema:"name"`
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

### Form validation with field-level errors

thx decodes form data into your `I` struct automatically but leaves validation to the handler. This gives you full control over how errors are presented — especially important for HTMX form submissions where you want to re-render the form with inline errors.

Use any validation library you prefer. Here's an example with `go-playground/validator`:

```go
import "github.com/go-playground/validator/v10"

var validate = validator.New(validator.WithRequiredStructEnabled())

type registerForm struct {
    Email    string `schema:"email"    validate:"required,email"`
    Password string `schema:"password" validate:"required,min=8"`
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

### Toast notifications (server-driven)

Inject a toast from any handler without client-side event listeners — use HTMX response headers to append a toast component to the page body:

```go
func (c *Controller) postSomething(ctx thx.Context, _ struct{}, form myForm) thx.Result {
    // ... do work ...

    hx := ctx.HTMX()
    hx.Retarget("body")
    hx.Reswap(thx.SwapBeforeEnd)
    return thx.Render(ctx, toastSuccess("Saved successfully"))
}
```

The toast templ component handles its own auto-dismiss (e.g. `setTimeout` to remove itself after 5 seconds). This pattern works from any handler and does not require a global trigger convention.

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
