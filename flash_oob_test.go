package thx

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func flashRenderer(flashes []FlashMessage) templ.Component {
	var parts []string
	for _, f := range flashes {
		parts = append(parts, f.Level+":"+f.Message)
	}
	return comp("<li>" + strings.Join(parts, "|") + "</li>")
}

func flashCookie(t *testing.T, flashes ...FlashMessage) *http.Cookie {
	t.Helper()
	data, err := json.Marshal(flashes)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &http.Cookie{
		Name:  flashCookieName,
		Value: base64.RawURLEncoding.EncodeToString(data),
	}
}

// flashRequest sends a request through a router configured with auto flash OOB.
func flashRequest(t *testing.T, req *http.Request, handler func(Context, struct{}) Result) *httptest.ResponseRecorder {
	t.Helper()
	router := New(WithFlashOOB("#flashes", SwapAfterBegin, flashRenderer,
		Get("/", handler),
	))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestFlashOOBAppendedToHTMXRender(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(flashCookie(t, FlashMessage{Level: "success", Message: "Saved!"}))

	rec := flashRequest(t, req, func(ctx Context, _ struct{}) Result {
		return Render(ctx, comp("<main>content</main>"))
	})

	body := rec.Body.String()
	want := `<main>content</main><template hx-swap-oob="afterbegin:#flashes"><li>success:Saved!</li></template>`
	if body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), flashCookieName+"=;") {
		t.Fatalf("flash cookie not cleared: %q", rec.Header().Values("Set-Cookie"))
	}
}

func TestFlashOOBIncludesFlashesSetInHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")

	rec := flashRequest(t, req, func(ctx Context, _ struct{}) Result {
		FlashError(ctx, "nope")
		return Render(ctx, comp("<main/>"))
	})

	if !strings.Contains(rec.Body.String(), "<li>error:nope</li>") {
		t.Fatalf("handler flash missing: %q", rec.Body.String())
	}
}

func TestFlashOOBAppendedAfterExplicitSwaps(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(flashCookie(t, FlashMessage{Level: "info", Message: "hi"}))

	rec := flashRequest(t, req, func(ctx Context, _ struct{}) Result {
		return SwapOOB(ctx, comp("<main/>"),
			OOBWithStrategy("#count", SwapInnerHTML, comp("3")),
		)
	})

	body := rec.Body.String()
	want := `<main/><template hx-swap-oob="innerHTML:#count">3</template>` +
		`<template hx-swap-oob="afterbegin:#flashes"><li>info:hi</li></template>`
	if body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestFlashOOBSkippedForNonHTMXRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(flashCookie(t, FlashMessage{Level: "info", Message: "hi"}))

	var got []FlashMessage
	rec := flashRequest(t, req, func(ctx Context, _ struct{}) Result {
		got = Flashes(ctx)
		return Render(ctx, comp("<main/>"))
	})

	if strings.Contains(rec.Body.String(), "hx-swap-oob") {
		t.Fatalf("unexpected OOB swap: %q", rec.Body.String())
	}
	if len(got) != 1 {
		t.Fatalf("handler flashes = %v, want 1 message", got)
	}
}

func TestFlashOOBSkippedOnHistoryRestore(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-History-Restore-Request", "true")
	req.AddCookie(flashCookie(t, FlashMessage{Level: "info", Message: "hi"}))

	rec := flashRequest(t, req, func(ctx Context, _ struct{}) Result {
		return Render(ctx, comp("<main/>"))
	})

	if strings.Contains(rec.Body.String(), "hx-swap-oob") {
		t.Fatalf("unexpected OOB swap: %q", rec.Body.String())
	}
}

func TestFlashOOBSkippedWithoutPendingFlashes(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")

	rec := flashRequest(t, req, func(ctx Context, _ struct{}) Result {
		return Render(ctx, comp("<main/>"))
	})

	if rec.Body.String() != "<main/>" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestFlashOOBLeavesCookieOnRedirect(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")

	rec := flashRequest(t, req, func(ctx Context, _ struct{}) Result {
		FlashSuccess(ctx, "Welcome back!")
		return ctx.Redirect("/dashboard")
	})

	cookies := strings.Join(rec.Header().Values("Set-Cookie"), " ")
	if strings.Contains(cookies, flashCookieName+"=;") {
		t.Fatalf("flash cookie cleared on redirect: %q", cookies)
	}
	if !strings.Contains(cookies, flashCookieName+"=") {
		t.Fatalf("flash cookie not written: %q", cookies)
	}
}

func TestFlashOOBOwnsFlashesInsideLayout(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(flashCookie(t, FlashMessage{Level: "info", Message: "hi"}))

	var inLayout []FlashMessage
	layout := func(inner templ.Component) templ.Component {
		return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			inLayout = Flashes(ViewContext(ctx))
			return inner.Render(ctx, w)
		})
	}

	router := New(WithFlashOOB("#flashes", SwapAfterBegin, flashRenderer,
		WithLayout(layout, Get("/", func(ctx Context, _ struct{}) Result {
			return Render(ctx, comp("<main/>"))
		})),
	))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if inLayout != nil {
		t.Fatalf("layout consumed flashes: %v", inLayout)
	}
	if !strings.Contains(rec.Body.String(), "<li>info:hi</li>") {
		t.Fatalf("flash OOB missing: %q", rec.Body.String())
	}
}
