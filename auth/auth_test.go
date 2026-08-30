package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-thx/thx"
	"github.com/go-thx/thx/internal"
)

type user struct {
	ID   int
	Role string
}

// authenticate installs the given user on every request, mimicking an
// application's auth middleware. A nil user leaves the request anonymous.
func authenticate(u *user) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			if u != nil {
				req = req.WithContext(thx.SetAuth(req.Context(), *u))
			}
			next.ServeHTTP(res, req)
		})
	}
}

func ok(ctx thx.Context, _ struct{}) thx.Result {
	return thx.Raw("ok")
}

// guarded builds a handler with a single guarded route at /private/.
func guarded(u *user, rule Rule[user], opts ...GuardOption) http.Handler {
	return thx.New(thx.WithMiddleware(authenticate(u),
		WithGuard("/private", thx.Routes{thx.Get("/", ok)}, rule, opts...),
	))
}

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(path string) *http.Request {
	return httptest.NewRequest(http.MethodGet, path, nil)
}

var isAdmin = Check(func(u user) bool { return u.Role == "admin" }, "admin only")

func TestGuardAllowsAuthenticated(t *testing.T) {
	rec := do(guarded(&user{ID: 1, Role: "admin"}, isAdmin), get("/private"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestGuardDeniesUnauthenticatedWith401(t *testing.T) {
	rec := do(guarded(nil, isAdmin), get("/private"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestGuardDeniesForbiddenWith403(t *testing.T) {
	rec := do(guarded(&user{ID: 1, Role: "member"}, isAdmin), get("/private"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestGuardUnauthenticatedRedirectCarriesCurrentPath(t *testing.T) {
	h := guarded(nil, nil,
		RedirectUnauthorized("/login"),
		RedirectWithCurrentPath("path"),
	)

	rec := do(h, get("/private/settings?tab=general"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	want := "/login?path=%2Fprivate%2Fsettings%3Ftab%3Dgeneral"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestGuardUnauthenticatedRedirectUsesHXRedirect(t *testing.T) {
	req := get("/private")
	req.Header.Set("HX-Request", "true")

	rec := do(guarded(nil, nil, RedirectUnauthorized("/login")), req)

	if got := rec.Header().Get("HX-Redirect"); got != "/login" {
		t.Fatalf("HX-Redirect = %q, want %q", got, "/login")
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want empty: a 3xx would be swapped into the fragment", got)
	}
}

func TestGuardForbiddenRedirect(t *testing.T) {
	h := guarded(&user{ID: 1, Role: "member"}, isAdmin, RedirectForbidden("/denied"))

	rec := do(h, get("/private"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/denied" {
		t.Fatalf("Location = %q, want %q", got, "/denied")
	}
}

func TestGuardOnForbiddenReceivesReason(t *testing.T) {
	h := guarded(&user{ID: 1, Role: "member"}, isAdmin,
		OnForbidden(func(_ thx.Context, err error) thx.Result {
			return thx.Status(http.StatusForbidden).Raw(Reason(err))
		}),
	)

	rec := do(h, get("/private"))

	if rec.Body.String() != "admin only" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "admin only")
	}
}

func TestGuardRuleErrorIs500(t *testing.T) {
	boom := Rule[user](func(thx.Context, user) error {
		return errors.New("database unreachable")
	})

	rec := do(guarded(&user{ID: 1}, boom), get("/private"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestGuardCoversNotFoundUnderPrefix(t *testing.T) {
	rec := do(guarded(nil, nil), get("/private/nope"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// ownsUser needs a path parameter, which is only available once the route
// matched — the case route-level rules exist for.
var ownsUser = Rule[user](func(ctx thx.Context, u user) error {
	if ctx.Param("id") != strconv.Itoa(u.ID) {
		return Forbidden("not your account")
	}
	return nil
})

func routeRuleHandler(u *user, opts ...GuardOption) http.Handler {
	handler := func(ctx Context[user], _ struct{}) thx.Result {
		return thx.Raw(strconv.Itoa(ctx.Auth().ID))
	}

	return thx.New(thx.WithMiddleware(authenticate(u),
		WithGuard("/private", thx.Routes{
			thx.Get("/users/{id}", Get(handler, ownsUser)),
		}, Authenticated[user](), opts...),
	))
}

func TestRouteRuleAllowsOwner(t *testing.T) {
	rec := do(routeRuleHandler(&user{ID: 7}), get("/private/users/7"))

	if rec.Code != http.StatusOK || rec.Body.String() != "7" {
		t.Fatalf("status = %d body = %q, want 200 %q", rec.Code, rec.Body.String(), "7")
	}
}

func TestRouteRuleDeniesOtherUser(t *testing.T) {
	rec := do(routeRuleHandler(&user{ID: 7}), get("/private/users/8"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRouteRuleInheritsGuardDenialHandling(t *testing.T) {
	rec := do(routeRuleHandler(&user{ID: 7}, RedirectForbidden("/denied")), get("/private/users/8"))

	if got := rec.Header().Get("Location"); got != "/denied" {
		t.Fatalf("Location = %q, want %q", got, "/denied")
	}
}

func TestRouteRuleAllowsWhenNoRules(t *testing.T) {
	// A route without rules keeps working: All() over no rules allows.
	handler := func(ctx Context[user], _ struct{}) thx.Result {
		return thx.Raw("ok")
	}

	h := thx.New(thx.WithMiddleware(authenticate(&user{ID: 1}),
		WithGuard("/private", thx.Routes{thx.Get("/", Get(handler))}, Authenticated[user]()),
	))

	if rec := do(h, get("/private")); rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func ctxFor(t *testing.T) thx.Context {
	t.Helper()
	return internal.NewContext(get("/"), httptest.NewRecorder())
}

func TestAllStopsAtFirstDenial(t *testing.T) {
	var reached bool
	second := Rule[user](func(thx.Context, user) error {
		reached = true
		return nil
	})

	err := All(isAdmin, second)(ctxFor(t), user{Role: "member"})

	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if reached {
		t.Fatal("second rule ran after a denial")
	}
}

func TestAllWithoutRulesAllows(t *testing.T) {
	if rule := All[user](); rule != nil {
		t.Fatal("All() should collapse to nil, which allows unconditionally")
	}
}

func TestAnyAllowsWhenOneRuleAllows(t *testing.T) {
	isOwner := Check(func(u user) bool { return u.ID == 1 }, "owner only")

	if err := Any(isAdmin, isOwner)(ctxFor(t), user{ID: 1, Role: "member"}); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestAnyJoinsReasonsWhenAllDeny(t *testing.T) {
	isOwner := Check(func(u user) bool { return u.ID == 1 }, "owner only")

	err := Any(isAdmin, isOwner)(ctxFor(t), user{ID: 2, Role: "member"})

	if got := Reason(err); got != "admin only or owner only" {
		t.Fatalf("reason = %q, want %q", got, "admin only or owner only")
	}
}

func TestAnyPropagatesInternalError(t *testing.T) {
	boom := Rule[user](func(thx.Context, user) error { return errors.New("boom") })

	err := Any(boom, isAdmin)(ctxFor(t), user{Role: "admin"})

	if err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want the internal error to abort evaluation", err)
	}
}

func TestAnyWithoutRulesDenies(t *testing.T) {
	if err := Any[user]()(ctxFor(t), user{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestNotInvertsDecision(t *testing.T) {
	rule := Not(isAdmin, "admins not allowed here")

	if err := rule(ctxFor(t), user{Role: "member"}); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	err := rule(ctxFor(t), user{Role: "admin"})
	if got := Reason(err); got != "admins not allowed here" {
		t.Fatalf("reason = %q, want %q", got, "admins not allowed here")
	}
}

func TestNotPropagatesInternalError(t *testing.T) {
	boom := Rule[user](func(thx.Context, user) error { return errors.New("boom") })

	if err := Not(boom, "nope")(ctxFor(t), user{}); errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want the internal error to abort evaluation", err)
	}
}

func TestSubjectReturnsAuthenticatedEntity(t *testing.T) {
	req := get("/")
	req = req.WithContext(thx.SetAuth(req.Context(), user{ID: 3, Role: "admin"}))
	ctx := internal.NewContext(req, httptest.NewRecorder())

	if got := Subject[user](ctx); got.ID != 3 {
		t.Fatalf("subject = %+v, want ID 3", got)
	}
}
