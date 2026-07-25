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

func TestCachedPartialHitSkipsFactory(t *testing.T) {
	var calls int32
	render := CachedPartial(time.Minute,
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

func TestCachedPartialSingleFlight(t *testing.T) {
	var calls int32
	render := CachedPartial(time.Minute,
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

func TestCachedPartialDistinctKeys(t *testing.T) {
	var calls int32
	lang := "en"
	render := CachedPartial(time.Minute,
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

func TestCachedPartialStaleReRenders(t *testing.T) {
	var calls atomic.Int32
	out := "v1"
	render := CachedPartial(20*time.Millisecond,
		func(context.Context) string { return "k" },
		func(context.Context) (templ.Component, error) {
			calls.Add(1)
			return comp(out), nil
		},
	)

	ctx := testContext()

	if got := writeBody(t, render(ctx)); got != "v1" {
		t.Fatalf("cold body: %q", got)
	}

	out = "v2"
	time.Sleep(40 * time.Millisecond) // let the entry go stale

	// Stale read blocks and re-renders synchronously to v2.
	if got := writeBody(t, render(ctx)); got != "v2" {
		t.Fatalf("stale body: %q, want v2 (re-rendered)", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("factory called %d times, want 2", calls.Load())
	}
}

func TestCachedPartialStaleServedOnError(t *testing.T) {
	var calls atomic.Int32
	fail := false
	render := CachedPartial(20*time.Millisecond,
		func(context.Context) string { return "k" },
		func(context.Context) (templ.Component, error) {
			calls.Add(1)
			if fail {
				return nil, errors.New("boom")
			}
			return comp("v1"), nil
		},
	)

	ctx := testContext()
	if got := writeBody(t, render(ctx)); got != "v1" {
		t.Fatalf("cold body: %q", got)
	}

	fail = true
	time.Sleep(40 * time.Millisecond)

	// Stale re-render fails, so the held stale bytes are served instead of 500.
	if got := writeBody(t, render(ctx)); got != "v1" {
		t.Fatalf("body on failed re-render: %q, want v1 (stale served)", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("factory called %d times, want 2 (re-render attempted)", calls.Load())
	}
}

func TestCachedPartialErrorNotCached(t *testing.T) {
	var calls int32
	fail := true
	boom := errors.New("boom")
	render := CachedPartial(time.Minute,
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

func TestCachedPartialCacheControl(t *testing.T) {
	render := CachedPartial(90*time.Second,
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
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type: %q", got)
	}
}

func TestCachedPartialNoCacheControlByDefault(t *testing.T) {
	render := CachedPartial(time.Minute,
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
