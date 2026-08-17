package snapshot

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/momja/Exhibit/internal/scanner"
)

// RuntimeAsset is one binary payload vendored out of a page, on its way to a
// blob of its own. It carries the absolute URL the page will ask for at run
// time, because that URL — not the bytes, and not any name derived from them —
// is what the render surface's manifest is keyed by.
type RuntimeAsset struct {
	SourceURL   string // resolved absolute URL the page fetches
	ContentType string // corrected where the source server was vague (see assetContentType)
	Body        []byte
}

// CollectRuntimeAssets fetches the binary payloads a page loads from
// JavaScript, which the markup walker by definition cannot see.
//
// The problem it solves is origin relocation. A page served from its own site
// fetches `/app.wasm` same-origin, and same-origin requests need no CORS
// headers — so source sites overwhelmingly do not send any. Relocated to the
// render origin (and, inside the sandbox, to an opaque origin) that same fetch
// becomes cross-origin and the browser refuses to read the response. The
// allowlist cannot fix it: the request is permitted, the *read* is what fails.
//
// It collects; it does not transform. The document comes back untouched, and
// the returned assets are stored as blobs of their own (av-20fk), with the
// render surface injecting the manifest that redirects each fetch. Two things
// follow from moving the substitution to render time, and both are the reason
// for it:
//
//   - the stored body keeps its original fetch literals, so an agent rewriting
//     the whole document — the normal operation in the preview loop — cannot
//     break asset loading, because there is nothing in the body to break;
//   - the assets are the single source of truth, rather than being copied into
//     every stored body as ~1.33x base64.
//
// It mirrors InlineHTMLAssets' contract on failure: a non-nil error means the
// document could not be parsed, while per-asset problems come back as
// []*FetchError and leave the page's reference untouched — so an asset that
// could not be vendored still surfaces in the footprint (and, over the cap, as
// an explained failure rather than a silent TypeError at render).
func CollectRuntimeAssets(ctx context.Context, f *Fetcher, body string) ([]RuntimeAsset, []*FetchError, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil, nil, err
	}

	c := &runtimeCollector{ctx: ctx, f: f, seen: map[string]bool{}}
	c.walk(doc)
	if len(c.assets) > 0 {
		slog.DebugContext(ctx, "snapshot runtime assets collected", slog.Int("assets", len(c.assets)))
	}
	return c.assets, c.errs, nil
}

// inlineExtensions are the path extensions the runtime pass will vendor.
// Deliberately narrow: these are opaque binary payloads a tool loads in order to
// function at all, and keying on them means the pass never speculatively GETs an
// endpoint that merely looks like a URL (an API path, a template, a route).
var inlineExtensions = map[string]bool{
	".wasm": true,
	".data": true,
	".bin":  true,
	".mem":  true,
}

type runtimeCollector struct {
	ctx    context.Context
	f      *Fetcher
	seen   map[string]bool // absolute URLs already attempted, successfully or not
	assets []RuntimeAsset
	errs   []*FetchError
}

// walk visits every <script> and harvests the references in its text. Only
// script text is considered, so a fetch( shown inside a <pre> code sample is
// documentation rather than a dependency and is left alone. Only fetch-call
// literals are harvested (scanner.FetchRefs): the manifest these assets feed is
// consulted by a window.fetch wrapper, and native ESM module loading never goes
// through window.fetch, so an import-derived entry could never be matched.
// Import refs stay with the footprint pass, where they feed the script-src
// allowlist instead.
func (c *runtimeCollector) walk(n *html.Node) {
	if n.Type == html.ElementNode && n.DataAtom == atom.Script {
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.TextNode {
				for _, ref := range scanner.FetchRefs(ch.Data) {
					c.consider(ref)
				}
			}
		}
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.walk(ch)
	}
}

// consider vendors one reference if it is fetchable, carries an inlinable
// extension, and fits the budget.
func (c *runtimeCollector) consider(ref string) {
	if !fetchable(ref) || !inlinableExt(ref) {
		return
	}
	abs, err := c.f.Resolve(ref)
	if err != nil {
		c.record(err)
		return
	}
	if c.seen[abs] {
		return
	}
	c.seen[abs] = true

	asset, err := c.f.FetchWithCap(c.ctx, ref, c.f.limits.MaxInlineAssetBytes)
	if err != nil {
		c.record(err)
		return
	}
	c.assets = append(c.assets, RuntimeAsset{
		SourceURL:   abs,
		ContentType: assetContentType(asset, abs),
		Body:        asset.Body,
	})
}

// record appends a fetch failure to the run's residual list.
func (c *runtimeCollector) record(err error) {
	var fe *FetchError
	if errors.As(err, &fe) {
		c.errs = append(c.errs, fe)
	}
}

// inlinableExt reports whether a reference's path ends in an extension the
// runtime pass vendors. Query and fragment are ignored so `/app.wasm?v=3`
// still matches.
func inlinableExt(ref string) bool {
	u, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return false
	}
	return inlineExtensions[strings.ToLower(path.Ext(u.Path))]
}

// assetContentType settles the type the render surface will serve these bytes
// under, forcing application/wasm for .wasm.
//
// WebAssembly's streaming entry points reject any response whose type is not
// exactly that, and neither the origin server's header nor mime.TypeByExtension
// is guaranteed to supply it — an octet-stream fallback would load the bytes and
// still fail to instantiate. Deciding it here rather than at serve time means
// the stored row is already correct and the render path has no special cases.
func assetContentType(asset *Asset, absURL string) string {
	if u, err := url.Parse(absURL); err == nil && strings.EqualFold(path.Ext(u.Path), ".wasm") {
		return "application/wasm"
	}
	if asset.ContentType != "" {
		return asset.ContentType
	}
	return "application/octet-stream"
}
