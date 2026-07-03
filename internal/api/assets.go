package api

import (
	"embed"
	"io/fs"
	"net/http"
)

// assetsFS embeds the esbuild-bundled client JS islands (built from web/editor
// by `make assets` and committed) so production serves them from the binary
// with no Node runtime.
//
//go:embed assets
var assetsFS embed.FS

// assetsHandler serves the embedded static assets under /assets/ on the app
// origin.
func assetsHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}
