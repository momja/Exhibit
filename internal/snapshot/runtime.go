package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/momja/Exhibit/internal/scanner"
)

// InlineRuntimeAssets vendors the binary payloads a page fetches from
// JavaScript at run time, which the markup walker cannot see.
//
// The problem it solves is origin relocation. A page served from its own site
// fetches `/app.wasm` same-origin, and same-origin requests need no CORS
// headers — so source sites overwhelmingly do not send any. Relocated to the
// render origin (and, inside the sandbox, to an opaque origin) that same fetch
// becomes cross-origin and the browser refuses to read the response. The
// allowlist cannot fix it: the request is permitted, the *read* is what fails.
// Vendoring the bytes removes the request entirely.
//
// Substitution is by interception, not by rewriting the JS. A literal in the
// source is a fragile thing to edit — minified, alt-quoted, recurring in
// unrelated contexts, or never present because the URL is assembled at run
// time. Instead the vendored bytes go into a manifest keyed by absolute URL and
// a small wrapper around window.fetch consults it at call time, which also
// catches URLs the page builds itself. The manifest values are data: URIs, so
// the synthetic response carries a real Content-Type — WebAssembly's streaming
// entry points reject anything that is not exactly application/wasm.
//
// It mirrors InlineHTMLAssets' contract: a non-nil error means only that the
// document could not be parsed or re-rendered, while per-asset problems come
// back as []*FetchError and leave their reference untouched, so an asset that
// could not be vendored still surfaces in the footprint (and, over the cap,
// as an explained failure rather than a silent TypeError at render).
func InlineRuntimeAssets(ctx context.Context, f *Fetcher, body string) (string, []*FetchError, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return body, nil, err
	}

	in := &runtimeInliner{ctx: ctx, f: f, manifest: map[string]string{}, seen: map[string]bool{}}
	in.walk(doc)
	if len(in.manifest) == 0 {
		return body, in.errs, nil
	}
	in.injectManifest(doc)

	var buf strings.Builder
	if err := html.Render(&buf, doc); err != nil {
		return body, in.errs, err
	}
	slog.DebugContext(ctx, "snapshot runtime assets inlined", slog.Int("assets", len(in.manifest)))
	return buf.String(), in.errs, nil
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

type runtimeInliner struct {
	ctx      context.Context
	f        *Fetcher
	manifest map[string]string // absolute URL -> data: URI
	seen     map[string]bool   // absolute URLs already attempted, successfully or not
	errs     []*FetchError
}

// walk visits every <script> and harvests the references in its text. Only
// script text is considered, so a fetch( shown inside a <pre> code sample is
// documentation rather than a dependency and is left alone. Only fetch-call
// literals are harvested (scanner.FetchRefs): the manifest is consulted by a
// window.fetch wrapper, and native ESM module loading never goes through
// window.fetch, so an import-derived entry could never be matched. Import
// refs stay with the footprint pass, where they feed the script-src
// allowlist instead.
func (in *runtimeInliner) walk(n *html.Node) {
	if n.Type == html.ElementNode && n.DataAtom == atom.Script {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				for _, ref := range scanner.FetchRefs(c.Data) {
					in.consider(ref)
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		in.walk(c)
	}
}

// consider vendors one reference if it is fetchable, carries an inlinable
// extension, and fits the budget.
func (in *runtimeInliner) consider(ref string) {
	if !fetchable(ref) || !inlinableExt(ref) {
		return
	}
	abs, err := in.f.Resolve(ref)
	if err != nil {
		in.record(err)
		return
	}
	if in.seen[abs] {
		return
	}
	in.seen[abs] = true

	asset, err := in.f.FetchWithCap(in.ctx, ref, in.f.limits.MaxInlineAssetBytes)
	if err != nil {
		in.record(err)
		return
	}
	in.manifest[abs] = runtimeDataURI(asset, abs)
}

// record appends a fetch failure to the run's residual list.
func (in *runtimeInliner) record(err error) {
	var fe *FetchError
	if errors.As(err, &fe) {
		in.errs = append(in.errs, fe)
	}
}

// injectManifest prepends the manifest and the fetch wrapper to <head> so they
// install before any of the page's own scripts run. html.Parse always
// synthesizes a <head>, so the lookup cannot fail on a parsed document.
func (in *runtimeInliner) injectManifest(doc *html.Node) {
	head := findElement(doc, atom.Head)
	if head == nil {
		return
	}
	// json.Marshal escapes <, > and & to their \u00NN forms, so no manifest
	// value can terminate the enclosing <script> element.
	payload, err := json.Marshal(in.manifest)
	if err != nil {
		return
	}
	script := &html.Node{Type: html.ElementNode, Data: "script", DataAtom: atom.Script}
	setText(script, manifestScript(string(payload)))
	head.InsertBefore(script, head.FirstChild)
}

// manifestScript renders the interceptor. It is deliberately tiny and total:
// anything it does not recognise — a non-GET, an unparseable input, a URL not in
// the manifest — falls through to the real fetch untouched, so installing it can
// only add behaviour, never remove any.
func manifestScript(manifestJSON string) string {
	return `
(function () {
  var M = ` + manifestJSON + `;
  var nativeFetch = window.fetch;
  if (typeof nativeFetch !== 'function') return;
  window.fetch = function (input, init) {
    try {
      // Only GET is served from the manifest: a POST to the same URL is a
      // different request and must reach the network.
      var method = (init && init.method) || (input && input.method) || 'GET';
      if (String(method).toUpperCase() === 'GET') {
        var raw = typeof input === 'string' ? input : (input && input.url) || String(input);
        var resolved = new URL(raw, document.baseURI).href;
        if (Object.prototype.hasOwnProperty.call(M, resolved)) {
          // The value is a data: URI, so this never touches the network and the
          // response carries the asset's real Content-Type.
          return nativeFetch(M[resolved]);
        }
      }
    } catch (e) { /* fall through to the real fetch */ }
    return nativeFetch(input, init);
  };
})();
`
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

// runtimeDataURI encodes a vendored asset, forcing application/wasm for .wasm.
// WebAssembly.instantiateStreaming rejects any response whose type is not
// exactly that, and neither the origin server's header nor mime.TypeByExtension
// is guaranteed to supply it — an octet-stream fallback would load the bytes and
// still fail to instantiate.
func runtimeDataURI(asset *Asset, absURL string) string {
	if u, err := url.Parse(absURL); err == nil && strings.EqualFold(path.Ext(u.Path), ".wasm") {
		forced := *asset
		forced.ContentType = "application/wasm"
		return dataURI(&forced)
	}
	return dataURI(asset)
}

// findElement returns the first element with the given tag in document order.
func findElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, a); found != nil {
			return found
		}
	}
	return nil
}
