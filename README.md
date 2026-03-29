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
    <a href="https://go.dev/doc/devel/release"><img src="https://img.shields.io/badge/go-v1.25.1-informational" alt="Go v1.25.1"></a>
    &nbsp;
    <a href="https://github.com/a-h/templ"><img src="https://img.shields.io/badge/go--templ-v0.3.977-informational" alt="Go-Templ v0.3.977"></a>
    &nbsp;
    <a href="https://github.com/bigskysoftware/htmx"><img src="https://img.shields.io/badge/htmx-v2.0.7-informational" alt="HTMX v2.0.7"></a>
    &nbsp;
</p>

## About

This library is a companion to the already awesome libraries go-templ and htmx.
It is the glue between those layers, making writing typesafe SSR-based web controllers a joy.

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
