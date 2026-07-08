package snapshot

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// InlineCSS rewrites relative url() and @import references in a CSS text into
// self-contained form: url() targets become data: URIs and @import'd sheets are
// fetched and folded in, recursively. References resolve against baseURL — the
// stylesheet's OWN absolute URL for a fetched sheet, or the document URL for an
// inline <style>/style= block — because an @import chain re-bases per sheet
// (exhibit-lwb.4). Every network fetch goes through the shared Fetcher f so the
// snapshot's byte/count budget and SSRF guard stay enforced in one place; bound
// @import recursion with the fetcher's asset-count budget plus a small nesting
// cap so a cyclic import chain cannot loop forever.
//
// It returns the transformed CSS plus a *FetchError for every reference that
// could not be inlined (over a limit, non-200, blocked). Those references
// survive verbatim in the output so the base-aware scanner (exhibit-lwb.5) can
// still surface their origin in the network footprint. A single failed inline
// never aborts the transform.
//
// The implementation is owned by exhibit-lwb.4. This function is the seam that
// exhibit-lwb.3's HTML inliner calls for <style> text, style= attributes, and
// fetched stylesheet bodies; its signature is fixed here so the two tickets can
// be built in parallel and compose without further coordination.
func InlineCSS(ctx context.Context, f *Fetcher, baseURL, css string) (string, []*FetchError) {
	ci := &cssInliner{ctx: ctx, f: f, path: make(map[string]bool)}
	out := ci.inline(baseURL, css, 0)
	return out, ci.errs
}

// maxImportDepth caps how deep an @import chain may nest before folding stops.
// The fetcher's asset-count/byte budget already bounds total work and cycle
// detection prevents loops; this is a defence-in-depth ceiling on pathological
// but acyclic chains so a linear import fan-out cannot recurse without limit.
const maxImportDepth = 8

var (
	errBadCSSRef     = errors.New("unresolvable CSS reference")
	errImportTooDeep = errors.New("@import nesting limit reached")
)

// cssRefRE matches, in one alternation, either a full @import statement or a
// standalone url() token. Ordering the @import alternative first means an
// @import's own url() is consumed as part of the statement (leftmost match)
// rather than matched a second time as a bare url().
//
// Import branch capture groups:
//
//	1 url("…")   2 url('…')   3 url(…)   4 "…"   5 '…'   6 media query text
//
// url() branch capture groups:
//
//	7 url("…")   8 url('…')   9 url(…)
//
// This is a deliberate heuristic (matching the project's stated approach to
// CSS/JS scanning), not a full CSS tokenizer: it does not skip comments or
// handle escaped delimiters inside url() tokens. The CSP allowlist remains the
// enforced boundary; whatever this misses stays verbatim for the scanner.
var cssRefRE = regexp.MustCompile(`(?i)` +
	`@import\s*(?:url\(\s*(?:"([^"]*)"|'([^']*)'|([^)"']*))\s*\)|"([^"]*)"|'([^']*)')\s*([^;]*);` +
	`|` +
	`url\(\s*(?:"([^"]*)"|'([^']*)'|([^)"']*))\s*\)`)

// cssInliner carries the shared state for one InlineCSS call: the fetcher and
// its budget, the accumulated per-reference failures, and the set of sheet URLs
// on the current @import path for cycle detection.
type cssInliner struct {
	ctx  context.Context
	f    *Fetcher
	errs []*FetchError
	path map[string]bool // absolute sheet URLs currently being folded (import stack)
}

// inline rewrites one sheet's body, resolving its references against base (the
// sheet's own absolute URL). depth is the sheet's position in the @import chain.
func (ci *cssInliner) inline(baseURL, css string, depth int) string {
	base, ok := absBase(baseURL)
	if !ok {
		// Without an absolute base we cannot resolve relative references; leave
		// the text untouched rather than emit noise. Callers pass the document
		// or sheet's absolute URL, so this is a defensive fallback.
		return css
	}
	baseKey := base.String()
	ci.path[baseKey] = true
	defer delete(ci.path, baseKey)

	var b strings.Builder
	last := 0
	for _, m := range cssRefRE.FindAllStringSubmatchIndex(css, -1) {
		b.WriteString(css[last:m[0]])
		whole := css[m[0]:m[1]]
		if m[12] != -1 { // group 6 (media) present ⇒ @import branch
			ref := group(css, m, 1, 2, 3, 4, 5)
			media := strings.TrimSpace(css[m[12]:m[13]])
			b.WriteString(ci.foldImport(base, ref, media, whole, depth))
		} else { // url() branch
			ref := group(css, m, 7, 8, 9)
			b.WriteString(ci.inlineURL(base, ref, whole))
		}
		last = m[1]
	}
	b.WriteString(css[last:])
	return b.String()
}

// inlineURL turns one url() reference into a data: URI, or leaves it verbatim.
func (ci *cssInliner) inlineURL(base *url.URL, ref, whole string) string {
	trimmed := strings.TrimSpace(ref)
	if skipRef(trimmed) {
		return whole // data:/blob:/about:/#fragment/empty — not a network fetch
	}
	abs, ok := resolveAbs(base, trimmed)
	if !ok {
		ci.errs = append(ci.errs, &FetchError{Ref: ref, Kind: ErrBadRef, Err: errBadCSSRef})
		return whole
	}
	asset, err := ci.f.Fetch(ci.ctx, abs)
	if err != nil {
		ci.record(ref, err)
		return whole
	}
	// Always double-quote: a data: URI can carry characters (';', spaces in a
	// media-type parameter) that would break an unquoted url() token.
	return `url("` + dataURI(asset) + `")`
}

// foldImport fetches an @import'd sheet, recursively inlines it against its own
// URL, and returns the folded rules (wrapped in @media when the import carried
// a media query). On any failure the original @import statement survives.
func (ci *cssInliner) foldImport(base *url.URL, ref, media, whole string, depth int) string {
	trimmed := strings.TrimSpace(ref)
	if skipRef(trimmed) {
		return whole
	}
	if depth+1 > maxImportDepth {
		ci.errs = append(ci.errs, &FetchError{Ref: ref, Kind: ErrBudget, Err: errImportTooDeep})
		return whole
	}
	abs, ok := resolveAbs(base, trimmed)
	if !ok {
		ci.errs = append(ci.errs, &FetchError{Ref: ref, Kind: ErrBadRef, Err: errBadCSSRef})
		return whole
	}
	if ci.path[abs] {
		// The sheet is already on the current import path: folding it again
		// would loop. Leave the back-edge verbatim; it is not a fetch failure.
		return whole
	}
	asset, err := ci.f.Fetch(ci.ctx, abs)
	if err != nil {
		ci.record(ref, err)
		return whole
	}
	inner := ci.inline(abs, string(asset.Body), depth+1)
	if media != "" {
		return "@media " + media + " {\n" + inner + "\n}"
	}
	return inner
}

// record appends a fetch failure, rewriting its Ref to the reference as it
// appeared in the CSS (the fetcher reports the absolute URL we handed it). The
// resolved URL is preserved so the scanner can surface the residual origin.
func (ci *cssInliner) record(ref string, err error) {
	var fe *FetchError
	if errors.As(err, &fe) {
		e := *fe
		e.Ref = ref
		ci.errs = append(ci.errs, &e)
		return
	}
	ci.errs = append(ci.errs, &FetchError{Ref: ref, Kind: ErrNetwork, Err: err})
}

// skipRef reports references that carry no network egress and must be left
// untouched with no error: empty refs, bare #fragments, and non-http(s) schemes
// such as data:, blob:, and about:.
func skipRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "#") {
		return true
	}
	if u, err := url.Parse(ref); err == nil {
		s := strings.ToLower(u.Scheme)
		if s != "" && s != "http" && s != "https" {
			return true
		}
	}
	return false
}

// absBase parses a caller-supplied base into an absolute http(s) URL.
func absBase(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, false
	}
	u.Fragment = ""
	return u, true
}

// resolveAbs resolves ref against base and returns the absolute http(s) URL.
// The fetcher resolves against its own base, so callers must hand it an already
// absolute URL to honour the per-sheet @import base.
func resolveAbs(base *url.URL, ref string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return "", false
	}
	abs := base.ResolveReference(u)
	if (abs.Scheme != "http" && abs.Scheme != "https") || abs.Host == "" {
		return "", false
	}
	abs.Fragment = ""
	return abs.String(), true
}

// group returns the first of the given submatch groups that participated in the
// match (its value), or "" if none did.
func group(s string, m []int, groups ...int) string {
	for _, g := range groups {
		if m[2*g] != -1 {
			return s[m[2*g]:m[2*g+1]]
		}
	}
	return ""
}
