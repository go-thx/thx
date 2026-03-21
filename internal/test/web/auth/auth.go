package auth

import (
	thxauth "github.com/go-thx/thx/auth"
	"thx.test/model"
)

type Context = thxauth.Context[model.User]
