package auth

import (
	"errors"
	"strings"

	"github.com/go-thx/thx"
	"github.com/go-thx/thx/internal"
)

// Rule decides whether an authenticated subject may proceed with a request.
// It returns nil to allow the request, an error wrapping ErrForbidden to deny
// it, and any other error to signal that the decision could not be made — the
// latter is treated as an internal failure, never as a silent denial.
//
// Rules only run for authenticated requests: the guard verifies authentication
// before evaluating them, so the subject is always populated.
type Rule[T any] func(ctx thx.Context, subject T) error

var (
	// ErrForbidden marks a denial by a Rule. Match it with errors.Is.
	ErrForbidden = errors.New("auth: forbidden")

	// ErrUnauthenticated marks a request without an authenticated subject.
	ErrUnauthenticated = errors.New("auth: unauthenticated")
)

// DeniedError is the error returned by a denying Rule. It carries a
// human-readable reason that denial handlers can render.
type DeniedError struct {
	Reason string
}

// Error implements the error interface.
func (e *DeniedError) Error() string {
	if e.Reason == "" {
		return ErrForbidden.Error()
	}
	return ErrForbidden.Error() + ": " + e.Reason
}

// Unwrap makes errors.Is(err, ErrForbidden) report true for denials.
func (e *DeniedError) Unwrap() error {
	return ErrForbidden
}

// Forbidden denies a request with the given reason.
func Forbidden(reason string) error {
	return &DeniedError{Reason: reason}
}

// Reason returns the denial reason carried by err, or "" if err is not a denial.
func Reason(err error) string {
	var denied *DeniedError
	if errors.As(err, &denied) {
		return denied.Reason
	}
	return ""
}

// Subject returns the authenticated entity stored on the context.
// It returns the zero value of T for unauthenticated requests.
func Subject[T any](ctx thx.Context) T {
	return internal.NewAuthContext[T](ctx).Auth()
}

// Authenticated is a Rule that allows every authenticated subject.
// Use it when a guard should require a login but no further authorization.
func Authenticated[T any]() Rule[T] {
	return nil
}

// Check builds a Rule from a predicate over the subject, denying with the
// given reason when the predicate returns false.
func Check[T any](predicate func(subject T) bool, reason string) Rule[T] {
	return func(_ thx.Context, subject T) error {
		if predicate(subject) {
			return nil
		}
		return Forbidden(reason)
	}
}

// All allows a request only if every rule allows it. Rules are evaluated in
// order and evaluation stops at the first denial or internal error.
// All with no rules always allows.
func All[T any](rules ...Rule[T]) Rule[T] {
	rules = compact(rules)
	if len(rules) == 0 {
		return nil
	}
	if len(rules) == 1 {
		return rules[0]
	}

	return func(ctx thx.Context, subject T) error {
		for _, rule := range rules {
			if err := rule(ctx, subject); err != nil {
				return err
			}
		}
		return nil
	}
}

// Any allows a request if at least one rule allows it. Internal errors are not
// swallowed: a rule that fails to reach a decision aborts the evaluation.
// Any with no rules always denies.
func Any[T any](rules ...Rule[T]) Rule[T] {
	if len(rules) == 0 {
		return func(thx.Context, T) error {
			return Forbidden("no rule granted access")
		}
	}

	// A nil rule allows unconditionally, which short-circuits the whole
	// disjunction.
	if len(rules) != len(compact(rules)) {
		return nil
	}

	return func(ctx thx.Context, subject T) error {
		reasons := make([]string, 0, len(rules))

		for _, rule := range rules {
			err := rule(ctx, subject)
			if err == nil {
				return nil
			}
			if !errors.Is(err, ErrForbidden) {
				return err
			}
			if reason := Reason(err); reason != "" {
				reasons = append(reasons, reason)
			}
		}

		return Forbidden(strings.Join(reasons, " or "))
	}
}

// Not inverts a rule: a request the rule allows is denied with the given
// reason, and a request it denies is allowed. Internal errors still abort.
func Not[T any](rule Rule[T], reason string) Rule[T] {
	return func(ctx thx.Context, subject T) error {
		if rule == nil {
			return Forbidden(reason)
		}

		err := rule(ctx, subject)
		if err == nil {
			return Forbidden(reason)
		}
		if errors.Is(err, ErrForbidden) {
			return nil
		}
		return err
	}
}

// compact drops nil rules, which allow unconditionally.
func compact[T any](rules []Rule[T]) []Rule[T] {
	out := make([]Rule[T], 0, len(rules))
	for _, rule := range rules {
		if rule != nil {
			out = append(out, rule)
		}
	}
	return out
}
