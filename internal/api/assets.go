package api

import (
	"embed"
	"io/fs"
	"net/http"
)

// assetsFS embeds the build-time frontend assets — the esbuild-bundled editor
// JS (web/editor) and the vendored Phosphor Icons CSS/webfont (web/icons) — so
// production serves them from the binary with no Node runtime. These are built
// by `make assets` (or the Dockerfile's Node stage), not committed to git; a
// checkout without them fails this go:embed at compile time by design.
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
