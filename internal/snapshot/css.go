package snapshot

import "context"

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
	// TODO(exhibit-lwb.4): implement recursive url()/@import inlining. Until
	// then this is the identity transform: callers compile and route CSS text
	// through the seam, and every url()/@import is left for the scanner to
	// surface as a residual origin.
	_ = ctx
	_ = f
	_ = baseURL
	return css, nil
}
