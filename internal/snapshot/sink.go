package snapshot

// Where a vendored markup asset ends up (av-oz40).
//
// The runtime pass (runtime.go) collects payloads and leaves the document
// alone, because the render surface can substitute them at call time through a
// `fetch` wrapper. The markup pass cannot borrow that trick: an `<img src>` is
// not loaded through `window.fetch`, so there is nothing to intercept and the
// reference itself has to change. That is the whole difference between the two,
// and it is why this seam exists only here.
//
// Rewriting markup is safe in a way rewriting the runtime pass's literals would
// not have been. That one's machinery is an injected *script*, which an agent
// rewriting the document could plausibly drop as noise; here the URL is the
// reference, an agent preserves an attribute like any other, and deleting the
// element is a deliberate act rather than breakage.

// AssetSink stores one vendored asset out of line and returns the URL the
// document should reference it by. Returning an error means the caller keeps
// the original reference — the page is then no worse off than it was before
// vendoring, which is the same contract every other failure here has.
type AssetSink func(RuntimeAsset) (string, error)

// InlineDataURIMaxBytes is the size at or below which a markup asset stays a
// `data:` URI rather than becoming an out-of-line asset.
//
// There is a real trade on each side, which is why this is a threshold rather
// than a rule. Inlining costs ~1.33x in the body — paid in the agent's context
// on every read and write, in the editor, and on the wire on every render,
// since the render document is necessarily no-store. Externalizing costs one
// extra HTTP request, which for a 200-byte favicon is plainly the worse deal.
//
// 8 KiB is chosen so the things that are numerous and tiny — icons, small
// sprites, a logo — stay inline and cost roughly nothing even in quantity
// (~11 KB each encoded), while anything large enough to actually matter in bulk
// leaves the document. The exact number is less important than that both
// failure modes were weighed; move it if real artifacts argue otherwise.
const InlineDataURIMaxBytes = 8 << 10

// place decides where one fetched asset lives and returns the reference the
// document should carry. With no sink configured everything is inlined, which
// is the behaviour every caller had before av-oz40 and what the CSS and markup
// tests exercise directly.
func place(sink AssetSink, asset *Asset) string {
	if sink == nil || len(asset.Body) <= InlineDataURIMaxBytes {
		return dataURI(asset)
	}
	url, err := sink(RuntimeAsset{
		SourceURL:   asset.URL,
		ContentType: assetContentType(asset, asset.URL),
		Body:        asset.Body,
	})
	if err != nil || url == "" {
		// Storing it failed; the bytes are still in hand, so inline them
		// rather than losing the asset. Degrading to the old behaviour beats
		// leaving a reference that resolves to nothing.
		return dataURI(asset)
	}
	return url
}
