package api

import (
	"embed"
	"io/fs"
	"net/http"
)

// embeddedAssets holds the build-time frontend assets compiled into the binary
// via the standard library's embed package, so production serves them under
// /assets/ with no Node runtime. The tree is produced by scripts/build-assets.sh
// (run by `make assets` or the Dockerfile's Node stage) and is not committed to
// git; a checkout without it fails this go:embed at compile time by design.
//
//go:embed assets
var embeddedAssets embed.FS

// assetsHandler serves the embedded static assets under /assets/ on the app
// origin.
func assetsHandler() http.Handler {
	sub, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}
