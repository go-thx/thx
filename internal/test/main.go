package main

import (
	"context"

	"thx.test/web"
	"thx.test/web/pages"
	"thx.test/web/pages/private"
	"thx.test/web/pages/public"
)

func main() {
	mainController := pages.New(
		public.New(),
		private.New(),
	)

	server := web.New(mainController)

	if err := server.Run(context.Background()); err != nil {
		panic(err)
	}
}
