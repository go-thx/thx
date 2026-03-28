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
