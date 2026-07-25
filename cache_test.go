package thx

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/go-thx/thx/internal"
)

func testContext() Context {
	return internal.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
}

func comp(html string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, html)
		return err
	})
}

func writeBody(t *testing.T, r Result) string {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := r.WriteResult(rec); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}
	return rec.Body.String()
}

func TestCachedRenderHitSkipsFactory(t *testing.T) {
	var calls int32
	render := CachedRender(time.Minute,
		func(context.Context) string { return "k" },
		func(context.Context) (templ.Component, error) {
			atomic.AddInt32(&calls, 1)
			return comp("hello"), nil
		},
	)

	ctx := testContext()
	if got := writeBody(t, render(ctx)); got != "hello" {
		t.Fatalf("body: %q", got)
	}
	if got := writeBody(t, render(ctx)); got != "hello" {
		t.Fatalf("body: %q", got)
	}
	if calls != 1 {
		t.Fatalf("factory called %d times, want 1", calls)
	}
}

func TestCachedRenderSingleFlight(t *testing.T) {
	var calls int32
	render := CachedRender(time.Minute,
		func(context.Context) string { return "k" },
		func(context.Context) (templ.Component, error) {
			atomic.AddInt32(&calls, 1)
			time.Sleep(20 * time.Millisecond)
			return comp("hi"), nil
		},
	)

	ctx := testContext()
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			render(ctx)
		}()
	}
	wg.Wait()

	if calls != 1 {
		t.Fatalf("factory called %d times under concurrency, want 1", calls)
	}
}

func TestCachedRenderDistinctKeys(t *testing.T) {
	var calls int32
	lang := "en"
	render := CachedRender(time.Minute,
		func(context.Context) string { return lang },
		func(context.Context) (templ.Component, error) {
			atomic.AddInt32(&calls, 1)
			return comp(lang), nil
		},
	)

	ctx := testContext()
	if got := writeBody(t, render(ctx)); got != "en" {
		t.Fatalf("body: %q", got)
	}
	lang = "de"
	if got := writeBody(t, render(ctx)); got != "de" {
		t.Fatalf("body: %q", got)
	}
	lang = "en"
	if got := writeBody(t, render(ctx)); got != "en" {
		t.Fatalf("cached body: %q", got)
	}

	if calls != 2 {
		t.Fatalf("factory called %d times, want 2 (one per key)", calls)
	}
}

func TestCachedRenderExpiry(t *testing.T) {
	var calls int32
	render := CachedRender(20*time.Millisecond,
		func(context.Context) string { return "k" },
		func(context.Context) (templ.Component, error) {
			atomic.AddInt32(&calls, 1)
			return comp("x"), nil
		},
	)

	ctx := testContext()
	render(ctx)
	time.Sleep(40 * time.Millisecond)
	render(ctx)

	if calls != 2 {
		t.Fatalf("factory called %d times, want 2 after expiry", calls)
	}
}

func TestCachedRenderErrorNotCached(t *testing.T) {
	var calls int32
	fail := true
	boom := errors.New("boom")
	render := CachedRender(time.Minute,
		func(context.Context) string { return "k" },
		func(context.Context) (templ.Component, error) {
			atomic.AddInt32(&calls, 1)
			if fail {
				return nil, boom
			}
			return comp("ok"), nil
		},
	)

	ctx := testContext()

	rec := httptest.NewRecorder()
	if err := render(ctx).WriteResult(rec); !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}

	fail = false
	if got := writeBody(t, render(ctx)); got != "ok" {
		t.Fatalf("body after retry: %q", got)
	}

	if calls != 2 {
		t.Fatalf("factory called %d times, want 2 (error not cached, retried)", calls)
	}
}

func TestCachedRenderCacheControl(t *testing.T) {
	render := CachedRender(90*time.Second,
		func(context.Context) string { return "k" },
		func(context.Context) (templ.Component, error) { return comp("x"), nil },
		WithCacheControl(),
	)

	rec := httptest.NewRecorder()
	if err := render(testContext()).WriteResult(rec); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=90" {
		t.Fatalf("Cache-Control: %q", got)
	}
}

func TestCachedRenderNoCacheControlByDefault(t *testing.T) {
	render := CachedRender(time.Minute,
		func(context.Context) string { return "k" },
		func(context.Context) (templ.Component, error) { return comp("x"), nil },
	)

	rec := httptest.NewRecorder()
	if err := render(testContext()).WriteResult(rec); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("Cache-Control set unexpectedly: %q", got)
	}
}
