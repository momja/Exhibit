---
id: exhibit-lwb
status: closed
deps: []
links: [av-3pq6]
created: 2026-06-30T02:57:10Z
type: epic
priority: 2
---
# Snapshot-on-import: vendor relative assets into self-contained artifacts

When importing from a source URL (artifacts.go:104), an imported page's relative URLs (src="js/app.js", /assets/x.png, CSS url(bg.png)) break at render time: the sandboxed iframe runs on RENDER_ORIGIN with an opaque origin, so relatives resolve against RENDER_ORIGIN/blob, not the source site. They 404 or hit the wrong origin.

This is option B from the relative-URL design discussion: instead of referencing the live original (option A: inject <base href> + base-aware scanner — lightweight but rots when the source dies), snapshot the page into a genuinely self-contained file by fetching and inlining its relative assets at ingest. This is the only approach that honors the product's core promise (PRD §1, architecture §1.1): 'it's just a file that renders identically in five years.' A nice side effect: a fully vendored artifact collapses its network footprint toward connect-src 'none'.

Scope: URL-ingest path only. Must degrade gracefully on partial failure (some assets 404) and respect size/depth limits so a runaway page can't balloon storage. Assets that genuinely can't be inlined (runtime-constructed fetch URLs, opt-out origins) fall through to the existing scan -> allowlist -> CSP seam, which means the scanner must become base-aware (it currently discards relatives at scanner.go:93).

## Design

Keep it option A's seam-compatible: vendoring is an ingest-time transform that runs after fetch, before Blob.put. Pull complexity down into a single asset-resolution component rather than scattering URL rewriting across handlers (Ousterhout). Resolve every relative ref against the stored source base, fetch with strict limits, inline as data-URIs (binary) or inline <script>/<style> (text). CSS needs recursive handling (url() and @import can chain). Anything not inlinable is reported as a residual origin into the existing footprint/allowlist flow. Last-resort fallback if vendoring is disabled/fails: option A's <base href> tag.

## Acceptance Criteria

A URL-imported page with relative CSS/JS/img/font references renders correctly in the sandboxed iframe with no external requests for those assets; residual non-inlinable origins appear in the network footprint; partial fetch failures produce a usable artifact + a report; size/depth limits enforced.


