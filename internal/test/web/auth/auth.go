package auth

import (
	"github.com/go-thx/thx/thxauth"
	"thx.test/model"
)

type Context = thxauth.Context[model.User]
