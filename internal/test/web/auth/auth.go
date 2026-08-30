package auth

import (
	"strconv"

	"github.com/go-thx/thx"
	thxauth "github.com/go-thx/thx/auth"
	"thx.test/model"
)

type (
	Context = thxauth.Context[model.User]
	Rule    = thxauth.Rule[model.User]
)

// IsAdmin allows only users with the admin role.
var IsAdmin Rule = thxauth.Check(func(user model.User) bool {
	return user.Role == "admin"
}, "admin only")

// OwnsUser allows a request only if the {id} path parameter refers to the
// authenticated user. It depends on a path parameter, so it can only run on
// the route itself — a guard evaluates its rules before the route is matched.
func OwnsUser(ctx thx.Context, user model.User) error {
	if ctx.Param("id") != strconv.Itoa(user.ID) {
		return thxauth.Forbidden("not your account")
	}

	return nil
}
