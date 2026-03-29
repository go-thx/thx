package assets

import "embed"

//go:embed *
var assetsFS embed.FS

func Assets() embed.FS {
	return assetsFS
}
