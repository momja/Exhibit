---
id: av-oz40
status: open
deps: [av-20fk]
links: [av-vnkt]
created: 2026-08-17T06:01:07Z
type: feature
priority: 2
assignee: Max Omdal
tags: [snapshot, render, storage, agent]
---
# Externalize markup-referenced assets too: images, fonts, stylesheets, scripts

[[av-20fk]] moves the vendorer's *runtime* payloads (`.wasm`, `.data`, `.bin`, `.mem`) out of the artifact body. It deliberately leaves the markup pass alone — `InlineHTMLAssets` and the CSS inliner still fold images, fonts, stylesheets, and scripts into the body as `data:` URIs.

That leaves the original problem half-solved. The markup pass has a smaller per-asset cap (`MaxAssetBytes`, 5 MiB) but no limit on *how many* assets it inlines, bounded only by `MaxTotalBytes` at 48 MiB. So a snapshot of an image-heavy page can put more base64 into an agent's context than a single wasm payload does, and it lands there the same way: `get_artifact` hands `a.body` to the model verbatim, `update_artifact` requires sending all of it back to change one line, and the edit page loads the whole thing into CodeMirror.

The render cost is identical too. The render document is `Cache-Control: no-store`, so every inlined image is re-transferred and re-gzipped on every view, including every iteration of the agent preview loop.

## Design

Store markup-referenced assets as out-of-line blobs, reusing everything [[av-20fk]] builds: the same table (defined there as holding *any* out-of-line asset, with the runtime pass as its only initial producer), the same `GET /a/<artifactID>/assets/<assetID>` route, the same lifecycle and generation rules, and the same export materialization ([[av-vnkt]]).

**The one genuine difference is how substitution happens.**

| Loaded via | Substitute by | Why |
|---|---|---|
| `window.fetch` (av-20fk) | manifest + `fetch` interception | survives minification; catches runtime-constructed URLs |
| markup / CSS references (this ticket) | rewrite the reference to the asset URL at ingest | not fetch-loaded, so there is nothing to intercept |

Rewriting is safe here in a way it would not have been for the runtime pass. av-20fk avoids touching the body because its machinery is an *injected script* that an agent doing a wholesale rewrite could plausibly drop as noise. A rewritten `<img src>` has no such failure mode: the URL is the reference, an agent preserves it like any other attribute, and if it deletes the element then the image being gone is precisely what it intended. There is no manifest to clobber.

**CSP.** Extends av-20fk's system-source argument to the directives these assets load under — `img-src`, `font-src`, `media-src`, `style-src`, `script-src` — with the same per-artifact path scoping and the same reasoning: identical bytes, different addressing, no new reach. Still never a `decision='allow'` row, still never shown in the allowlist editor.

**Export.** [[av-vnkt]] already materializes out-of-line assets back into a single file. This adds cases to that one function — rewrite the reference back to a `data:` URI — rather than a second export path. The static build export ([[Exh-imom]]) benefits more: it emits assets as relative files beside the HTML, which is what a browser wants anyway.

**Scope boundary.** Recursive CSS (`@import` chains, nested `url()`) already resolves to a flat set of fetched assets in the existing inliner; each becomes a row. A stylesheet that is itself externalized still needs its own inner references rewritten, so the rewrite is applied to inlined CSS text as well as to markup attributes.

**Not in scope:** migrating artifacts that already carry inlined `data:` URIs, same as av-20fk. Those keep working untouched.

## Acceptance Criteria

- A URL ingest with `snapshot: true` of an image-heavy page stores each image as its own blob; the artifact body contains asset URLs and no base64 image payloads.
- The artifact renders identically to the inlined version, in a real browser, with an empty user allowlist.
- `get_artifact` on that artifact returns a body an agent can work with, and an agent round-trip preserves every image.
- A second view of the artifact does not re-transfer its images.
- Fonts referenced from inlined CSS, and `url()` references inside an externalized stylesheet, resolve correctly.
- The emitted CSP carries the per-artifact asset path in `img-src`, `font-src`, `media-src`, `style-src`, and `script-src`; `artifact_network_origins` gains no row.
- Export ([[av-vnkt]]) produces a single file that opens from `file://` with the service stopped, with images intact and no reference to `RENDER_ORIGIN`.
- Artifacts already carrying inlined `data:` URIs continue to render unchanged.
- `architecture.md` §3.4a records the second substitution mechanism (rewrite at ingest) beside the first (fetch interception), and the CSP directive list in §3.2 is updated.

