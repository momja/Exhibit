package api

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

// embeddedAssets holds the build-time frontend assets compiled into the binary
// via the standard library's embed package, so production serves them under
// /assets/ with no Node runtime. The tree is produced by scripts/build-assets.sh
// (run by `make assets` or the Dockerfile's Node stage) and is not committed to
// git; a checkout without it fails this go:embed at compile time by design.
//
//go:embed assets
var embeddedAssets embed.FS

// Pages reference assets through assetURL, which puts them under a path naming
// the build: /assets/v/<version>/gallery/index.js. The version is a hash of
// every embedded file, so any change to any asset moves every URL, and a URL
// that exists names one set of bytes forever. That is what lets those responses
// be cached as immutable: a relaunched page reuses them without asking the
// server whether they changed, which is a round trip per file — several in a
// row for an ES module's import chain — on a phone's network (Exh-hl64).
//
// Versioning the whole tree rather than each file is what keeps native ES
// modules working with no bundler: a module's relative import resolves inside
// the same /assets/v/<version>/ prefix, so it is versioned too without anyone
// rewriting it.
//
// The unversioned /assets/... paths still serve, for anything that names a
// file directly (the manifest's icons, a stylesheet's absolute url()), with a
// content ETag and no-cache: they revalidate rather than refetch.
//
// One binary holds one tree, so a page rendered before a deploy that asks for
// a module afterward gets the new build's bytes. That skew is not new — the
// unversioned URLs always had it — and the fallback below keeps those bytes
// out of every cache under the old version's name.
const versionedAssetPrefix = "/assets/v/"

var assetFiles, assetVersion = hashAssets()

// assetURL is the URL a page should use for an embedded asset, given its path
// below /assets/ (e.g. "gallery/index.js").
func assetURL(name string) string {
	return versionedAssetPrefix + assetVersion + "/" + name
}

func assetsFS() fs.FS {
	sub, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}

// hashAssets returns each embedded file's content ETag and the version of the
// whole tree.
func hashAssets() (map[string]string, string) {
	etags := map[string]string{}
	tree := sha256.New()
	err := fs.WalkDir(assetsFS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(assetsFS(), name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		// Weak, because the compressor may gzip the response: the bytes on
		// the wire differ by encoding while the content does not, and a
		// strong validator would let If-Range splice ranges of the two.
		etags[name] = `W/"` + hex.EncodeToString(sum[:16]) + `"`
		tree.Write([]byte(name + "\x00"))
		tree.Write(sum[:])
		return nil
	})
	if err != nil {
		panic(err)
	}
	return etags, hex.EncodeToString(tree.Sum(nil)[:6])
}

// assetsHandler serves the embedded static assets under /assets/ on the app
// origin.
func assetsHandler() http.Handler {
	files := http.FileServer(http.FS(assetsFS()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/assets/")
		cache := "no-cache"
		if rest, ok := strings.CutPrefix(r.URL.Path, versionedAssetPrefix); ok {
			version, file, _ := strings.Cut(rest, "/")
			name = file
			// A page rendered before a deploy still asks for the old version.
			// Only the current bytes exist, so it gets those, but they must not
			// be cached under a URL that promised different ones.
			if version == assetVersion {
				cache = "public, max-age=31536000, immutable"
			}
		}
		etag, ok := assetFiles[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", cache)
		// http.FileServer answers If-None-Match from this header.
		w.Header().Set("ETag", etag)
		r2 := new(http.Request)
		*r2 = *r
		u := *r.URL
		u.Path = "/" + name
		u.RawPath = ""
		r2.URL = &u
		files.ServeHTTP(w, r2)
	})
}
