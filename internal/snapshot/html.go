package snapshot

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"mime"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// InlineHTMLAssets is the HTML half of the snapshot transform (exhibit-lwb.3).
// It parses body with the same tokenizer the scanner uses, walks the node tree,
// and folds each fetchable asset reference into the document so the stored
// artifact is self-contained:
//
//   - <img src>, <source src>, and every candidate in <img>/<source> srcset
//     become data: URIs;
//   - <link rel=icon> / rel=apple-touch-icon hrefs become data: URIs;
//   - <script src> becomes an inline <script> carrying the fetched JS (src dropped);
//   - <link rel=stylesheet> becomes an inline <style> carrying the fetched CSS.
//
// CSS text is never rewritten here — it is routed through InlineCSS (the seam
// exhibit-lwb.4 implements) so url()/@import recursion lives in one place: a
// fetched stylesheet re-bases against its OWN absolute URL, while inline <style>
// bodies and style="" attributes re-base against the document URL.
//
// Navigation targets — <a href> and <form action> — are left untouched; they
// are follow-on requests, not asset fetches. Any reference that fails to fetch
// or exceeds the fetcher's limits keeps its original value and contributes a
// *FetchError to the returned slice, so the caller can report it and the
// base-aware scanner (exhibit-lwb.5) can still surface its origin. References
// that carry no network egress (data:, blob:, javascript:, mailto:, fragments,
// …) are skipped silently.
//
// A non-nil error is returned only for an unrecoverable parse or serialization
// failure; per-asset failures come back in the slice, never as the error.
func InlineHTMLAssets(ctx context.Context, f *Fetcher, body string) (string, []*FetchError, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return "", nil, err
	}

	in := &inliner{ctx: ctx, f: f, docBase: documentBase(f)}
	in.walk(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", nil, err
	}
	return buf.String(), in.errs, nil
}

// inliner carries the per-run state for one InlineHTMLAssets call: the fetcher
// (which owns budgets, dedupe, and the SSRF guard), the document base for
// re-basing inline CSS, and the accumulated per-asset failures.
type inliner struct {
	ctx     context.Context
	f       *Fetcher
	docBase string
	errs    []*FetchError
}

// documentBase returns the imported document's own absolute URL, used as the
// base for inline <style>/style="" CSS. Resolving an empty fragment folds it
// against the fetcher's base and yields that base unchanged.
func documentBase(f *Fetcher) string {
	base, err := f.Resolve("#")
	if err != nil {
		return ""
	}
	return base
}

func (in *inliner) walk(n *html.Node) {
	if n.Type == html.ElementNode {
		in.transform(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		in.walk(c)
	}
}

// transform applies the element-specific inlining rules, then the element-
// agnostic style="" rule. It runs before recursion; the text children it adds
// to <script>/<style> elements are inert to the walk.
func (in *inliner) transform(n *html.Node) {
	switch n.Data {
	case "img", "source":
		in.inlineAttr(n, "src")
		in.inlineSrcset(n)
	case "script":
		in.inlineScript(n)
	case "link":
		in.inlineLink(n)
	case "style":
		in.inlineStyleElement(n)
	}
	in.inlineStyleAttr(n)
}

// inlineAttr replaces the named attribute's value with a data: URI of the
// fetched asset. A failed or non-fetchable reference is left in place.
func (in *inliner) inlineAttr(n *html.Node, key string) {
	for i := range n.Attr {
		a := &n.Attr[i]
		if a.Namespace != "" || a.Key != key {
			continue
		}
		if uri, ok := in.toDataURI(a.Val); ok {
			a.Val = uri
		}
	}
}

// inlineSrcset rewrites each candidate URL in a srcset attribute to a data:
// URI, preserving descriptors. Candidates that fail to fetch keep their URL.
func (in *inliner) inlineSrcset(n *html.Node) {
	for i := range n.Attr {
		a := &n.Attr[i]
		if a.Namespace != "" || a.Key != "srcset" {
			continue
		}
		candidates := parseSrcset(a.Val)
		if len(candidates) == 0 {
			continue
		}
		var b strings.Builder
		for j, c := range candidates {
			if j > 0 {
				b.WriteString(", ")
			}
			ref := c.url
			if uri, ok := in.toDataURI(ref); ok {
				ref = uri
			}
			b.WriteString(ref)
			if c.descriptor != "" {
				b.WriteByte(' ')
				b.WriteString(c.descriptor)
			}
		}
		a.Val = b.String()
	}
}

// inlineScript replaces a <script src> with an inline script carrying the
// fetched JS text and drops the src attribute. Inline scripts (no src) and
// unfetchable/failed refs are left untouched.
func (in *inliner) inlineScript(n *html.Node) {
	src, ok := attrValue(n, "src")
	if !ok || !fetchable(src) {
		return
	}
	asset, err := in.f.Fetch(in.ctx, src)
	if err != nil {
		in.record(err)
		return
	}
	removeAttr(n, "src")
	setText(n, string(asset.Body))
}

// inlineLink turns a <link rel=stylesheet> into an inline <style> and a
// <link rel=icon>/rel=apple-touch-icon href into a data: URI. Other link
// relations (preload, manifest, alternate, …) are left untouched.
func (in *inliner) inlineLink(n *html.Node) {
	href, ok := attrValue(n, "href")
	if !ok {
		return
	}
	switch linkKind(attrValueOr(n, "rel", "")) {
	case linkStylesheet:
		if !fetchable(href) {
			return
		}
		asset, err := in.f.Fetch(in.ctx, href)
		if err != nil {
			in.record(err)
			return
		}
		// A fetched sheet re-bases against its own absolute URL, not the doc.
		css, errs := InlineCSS(in.ctx, in.f, asset.URL, string(asset.Body))
		in.errs = append(in.errs, errs...)
		n.Data = "style"
		n.DataAtom = atom.Style
		n.Attr = nil
		setText(n, css)
	case linkIcon:
		in.inlineAttr(n, "href")
	}
}

// inlineStyleElement routes an inline <style> block's text through InlineCSS,
// re-basing against the document URL.
func (in *inliner) inlineStyleElement(n *html.Node) {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	css := sb.String()
	if css == "" {
		return
	}
	out, errs := InlineCSS(in.ctx, in.f, in.docBase, css)
	in.errs = append(in.errs, errs...)
	setText(n, out)
}

// inlineStyleAttr routes a style="" attribute's value through InlineCSS,
// re-basing against the document URL. Applies to any element.
func (in *inliner) inlineStyleAttr(n *html.Node) {
	for i := range n.Attr {
		a := &n.Attr[i]
		if a.Namespace != "" || a.Key != "style" || a.Val == "" {
			continue
		}
		out, errs := InlineCSS(in.ctx, in.f, in.docBase, a.Val)
		in.errs = append(in.errs, errs...)
		a.Val = out
	}
}

// toDataURI fetches ref and returns its data: URI. On a non-fetchable ref it
// returns false without recording anything; on a fetch failure it records the
// *FetchError and returns false so the caller keeps the original reference.
func (in *inliner) toDataURI(ref string) (string, bool) {
	if !fetchable(ref) {
		return "", false
	}
	asset, err := in.f.Fetch(in.ctx, ref)
	if err != nil {
		in.record(err)
		return "", false
	}
	return dataURI(asset), true
}

// record appends a fetch failure to the run's residual list.
func (in *inliner) record(err error) {
	var fe *FetchError
	if errors.As(err, &fe) {
		in.errs = append(in.errs, fe)
	}
}

// dataURI encodes an asset as data:<content-type>;base64,<bytes>. When the
// server sent no Content-Type it is guessed from the URL's file extension.
// Spaces are stripped so the URI is safe to place inside a srcset attribute,
// where whitespace is significant.
func dataURI(asset *Asset) string {
	ct := strings.ReplaceAll(strings.TrimSpace(asset.ContentType), " ", "")
	if ct == "" {
		ct = guessContentType(asset.URL)
	}
	enc := base64.StdEncoding.EncodeToString(asset.Body)
	return "data:" + ct + ";base64," + enc
}

// guessContentType maps a URL's file extension to a MIME type, falling back to
// application/octet-stream when the extension is unknown or absent.
func guessContentType(rawURL string) string {
	ext := path.Ext(rawURL)
	if u, err := url.Parse(rawURL); err == nil {
		ext = path.Ext(u.Path)
	}
	if ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			return strings.ReplaceAll(ct, " ", "")
		}
	}
	return "application/octet-stream"
}

// fetchable reports whether a reference should be fetched and inlined. It
// admits relative, root-relative, protocol-relative, and http(s) URLs and
// rejects fragments and non-network schemes (data:, blob:, javascript:,
// mailto:, tel:, …) so those are left untouched without a spurious failure.
func fetchable(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return false
	}
	if strings.HasPrefix(ref, "//") {
		return true // protocol-relative
	}
	u, err := url.Parse(ref)
	if err != nil {
		return false
	}
	if u.Scheme == "" {
		return true // relative or root-relative
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

type linkRel int

const (
	linkOther linkRel = iota
	linkStylesheet
	linkIcon
)

// linkKind classifies a link's space-separated, case-insensitive rel tokens.
// stylesheet wins over icon if both somehow appear.
func linkKind(rel string) linkRel {
	kind := linkOther
	for _, tok := range strings.Fields(strings.ToLower(rel)) {
		switch tok {
		case "stylesheet":
			return linkStylesheet
		case "icon", "apple-touch-icon":
			kind = linkIcon
		}
	}
	return kind
}

type srcsetCandidate struct {
	url        string
	descriptor string
}

// parseSrcset splits a srcset attribute into its candidates following the
// WHATWG algorithm's shape: skip leading whitespace/commas, take the URL as a
// run of non-whitespace (trailing commas mark a descriptor-less candidate),
// then take the descriptor up to the next comma. Descriptors (Nw / Nx) never
// contain commas, so this stays simple.
func parseSrcset(input string) []srcsetCandidate {
	isWS := func(b byte) bool {
		switch b {
		case ' ', '\t', '\n', '\r', '\f':
			return true
		}
		return false
	}
	var out []srcsetCandidate
	i, n := 0, len(input)
	for i < n {
		for i < n && (isWS(input[i]) || input[i] == ',') {
			i++
		}
		if i >= n {
			break
		}
		start := i
		for i < n && !isWS(input[i]) {
			i++
		}
		u := input[start:i]
		var desc string
		if strings.HasSuffix(u, ",") {
			u = strings.TrimRight(u, ",")
		} else {
			for i < n && isWS(input[i]) {
				i++
			}
			ds := i
			for i < n && input[i] != ',' {
				i++
			}
			desc = strings.TrimSpace(input[ds:i])
		}
		if u != "" {
			out = append(out, srcsetCandidate{url: u, descriptor: desc})
		}
	}
	return out
}

// setText replaces a node's children with a single raw text node. For the
// raw-text elements this package targets (<script>, <style>) html.Render emits
// that text verbatim, so the fetched JS/CSS survives unescaped.
func setText(n *html.Node, text string) {
	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}
	n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
}

func attrValue(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func attrValueOr(n *html.Node, key, def string) string {
	if v, ok := attrValue(n, key); ok {
		return v
	}
	return def
}

func removeAttr(n *html.Node, key string) {
	dst := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			continue
		}
		dst = append(dst, a)
	}
	n.Attr = dst
}
