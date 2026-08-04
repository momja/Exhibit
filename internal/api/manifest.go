package api

import (
	"encoding/json"
	"net/http"

	"github.com/momja/Exhibit/internal/color"
)

// webManifestIcon is one entry of the web app manifest's "icons" array
// (https://developer.mozilla.org/docs/Web/Manifest/icons). json field names
// follow the spec's own snake_case, not Go convention.
type webManifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose,omitempty"`
}

// webManifest is the app-origin PWA manifest (av-fdcx). It is served as a
// static document — every field is fixed at build time, there is no
// per-request data — so manifestHandler marshals it once.
type webManifest struct {
	Name            string            `json:"name"`
	ShortName       string            `json:"short_name"`
	Icons           []webManifestIcon `json:"icons"`
	ThemeColor      string            `json:"theme_color"`
	BackgroundColor string            `json:"background_color"`
	Display         string            `json:"display"`
	StartURL        string            `json:"start_url"`
}

// manifestBackgroundColor matches the white card the logo's own artwork sits
// on (logo.svg) so the manifest's background_color agrees with the icons
// it's paired with.
const manifestBackgroundColor = "#ffffff"

var exhibitManifest = webManifest{
	Name:      "Exhibit",
	ShortName: "Exhibit",
	Icons: []webManifestIcon{
		{Src: "/assets/icons/icon-192.png", Sizes: "192x192", Type: "image/png", Purpose: "any"},
		{Src: "/assets/icons/icon-512.png", Sizes: "512x512", Type: "image/png", Purpose: "any"},
		{Src: "/assets/icons/icon-512-maskable.png", Sizes: "512x512", Type: "image/png", Purpose: "maskable"},
	},
	ThemeColor:      color.BrandBlue,
	BackgroundColor: manifestBackgroundColor,
	Display:         "standalone",
	StartURL:        "/",
}

var exhibitManifestJSON []byte

func init() {
	b, err := json.Marshal(exhibitManifest)
	if err != nil {
		panic(err) // exhibitManifest is a fixed literal; a marshal error here is a code bug
	}
	exhibitManifestJSON = b
}

// manifest serves GET /manifest.json on the app origin (av-fdcx). It is
// static and public — same trust level as the favicon — so it needs no auth
// and no per-request store lookup.
func (ro *Router) manifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Write(exhibitManifestJSON)
}
