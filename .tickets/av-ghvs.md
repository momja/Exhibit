---
id: av-ghvs
status: in_progress
deps: []
links: [av-ombn, av-f9b2, av-lh4a, av-b17a, av-wu9d, av-dwe2]
created: 2026-08-11T18:19:27Z
type: bug
priority: 1
assignee: Max Omdal
tags: [ingest, snapshot, render, csp, cors]
---
# URL-ingested artifacts break on runtime-fetched cross-origin assets (CORS), and the allowlist cannot fix it

A URL-ingested artifact that fetches an asset at runtime via JS (rather than referencing it from markup) fails with `TypeError: Failed to fetch`, and there is nothing the user can do about it. The network-allowlist — the only lever the UI offers — is already correct and has no effect, because the failure is CORS, not CSP.

Live repro: artifact 76fe2e49-e1f8-4aa8-bc14-b353fefb5411 on the maxomdal.com deployment, ingested from https://pokeemerald.com/. The page never boots; the on-screen error is:

    TypeError: Failed to fetch at wasmModule (.../a/76fe2e49...:1744:25) at boot (...:2097:35)

Reproduced locally end-to-end (fresh instance, `POST /api/artifacts {url, snapshot:true}` + allowlist PATCH). The locally ingested body and generated CSP are identical to production's in every relevant respect.

## The failing call

The artifact's loader does a plain root-relative fetch:

    async function wasmModule() {
      wasmModulePromise ??= fetch('/build/wasm/pokeemerald.wasm', { cache: 'no-store' })
        .then((res) => res.arrayBuffer())
        .then(async (bytes) => ({ bytes, module: await WebAssembly.compile(bytes) }));
      return wasmModulePromise;
    }

## Evidence: this is CORS, not CSP

Run inside the render origin (http://localhost:8081/a/<id>):

    document.baseURI  -> "https://pokeemerald.com/"                              (<base href> fallback working as designed)
    resolved URL      -> "https://pokeemerald.com/build/wasm/pokeemerald.wasm"   (correct)

    fetch(url, {mode:'no-cors'}) -> SUCCESS, type=opaque, status=0   <-- CSP PERMITS the request
    fetch(url)                   -> FAILED "Failed to fetch"         <-- CORS blocks READING it
    securitypolicyviolation events fired: 0                          <-- CSP never triggered

The generated CSP already contains `connect-src ... https://pokeemerald.com`, and the origin is on the allowlist. Confirmed server-side — the asset is served with no CORS headers whatsoever, for any Origin:

    curl -D - -H "Origin: null" https://pokeemerald.com/build/wasm/pokeemerald.wasm
      HTTP/2 200 / content-type: application/wasm / content-length: 12222529
      (no access-control-allow-origin, for `Origin: null` or for the render origin)

## Root cause

An origin-relocation problem, not a policy problem. The chain:

1. `internal/snapshot/html.go` vendors only assets referenced from *markup* — `img`/`source`, `script[src]`, `link` (html.go:94-99). A URL fetched at runtime by JS is invisible to it, so the 12 MiB `.wasm` is never vendored. (It would also be refused anyway: `MaxAssetBytes` is 5 MiB, `MaxTotalBytes` 20 MiB — fetcher.go:42-44.)
2. The scanner reports *origins*, never paths, so `/build/wasm/pokeemerald.wasm` never appears in the ingest footprint. The user is never told this dependency exists.
3. The `<base href="https://pokeemerald.com/">` fallback then resolves that relative URL back to the source site — which is the correct and intended behavior.
4. But the artifact now executes on a **different origin** (the render origin, or an opaque `null` origin inside the sandbox). A fetch that was **same-origin** on pokeemerald.com is now **cross-origin** — and same-origin fetches never need CORS headers, so source sites overwhelmingly do not send them.

So relocating the file silently converts a working same-origin fetch into a cross-origin one that the third party has no reason to permit. Every non-vendored runtime fetch in a URL-ingested artifact is exposed to this; the wasm case is just the most visible because nothing renders at all.

## Why this is worth fixing beyond this one artifact

The failure is **misattributed and unactionable**. It presents as a network error; the product's answer to network errors is the allowlist; the allowlist is already correct and changing it does nothing. A user can approve the origin repeatedly and never learn that CORS — something entirely outside their control — is the actual wall. This is the one failure mode the scan/approve/allowlist model does not cover and does not explain.

## Design

Three candidate directions, not mutually exclusive. (1) is cheap and should land regardless.

**1. Diagnose it (small, high value).** Make the failure legible instead of misleading. When a render-origin fetch fails CORS-style, the user should be told the origin is allowed but the third-party server refused cross-origin reads, and that the allowlist cannot fix it. Options: a note in the allowlist editor for URL-ingested artifacts, and/or an ingest-time warning when a snapshot leaves residual origins on a `source_url` artifact. Cheapest honest version is documentation plus a UI hint; there is no reliable in-page hook to catch a third party's missing CORS headers, so this is a heuristic explanation, not detection.

**2. Vendor runtime-fetched assets (partial, bounded).** Extend the vendorer past markup refs by reusing the scanner's literal-URL heuristic to catch string literals that look like asset paths, and inline them. Honest limits: this is a heuristic (`observe, don't predict` — architecture.md §1.4 — argues against leaning on it), it cannot see constructed URLs, and it does not help here anyway without a large bump to `MaxAssetBytes` (12 MiB for this one file). A 12 MiB `data:` URI is also base64-inflated to ~16 MiB in the stored body. Probably worth doing for small assets; it does not solve the wasm class.

**3. Same-origin asset proxy (real fix, biggest commitment).** Have the render surface serve vendored-by-reference assets from the render origin — e.g. `RENDER_ORIGIN/a/:id/_asset?u=<allowlisted-url>` — so the artifact's fetch becomes same-origin and CORS never applies. This genuinely fixes the class, but it is a significant policy change: the server would fetch on the artifact's behalf, which puts egress back on the service (SSRF surface, bandwidth, caching) and weakens the "the browser is the enforcer" property in architecture.md §4. It would need the existing `internal/snapshot` SSRF guard and the per-artifact allowlist enforced server-side on every proxied URL. Do not build this without an explicit decision — it is arguably a scope change to the security model.

Note that a large asset cannot become "just a file" in any case; option 3 keeps the artifact dependent on the live source site, which trades the §9 "no live-linked imports" non-goal against usability. That tension should be settled before building.

## Acceptance Criteria

- The CORS failure mode is documented (docs/security.md or docs/api.md ingest section): relocating a page to the render origin turns its same-origin runtime fetches into cross-origin ones, which fail unless the source server sends CORS headers, and the network allowlist cannot influence this.
- A user hitting this on a URL-ingested artifact is given an explanation that distinguishes "origin not allowlisted" (fixable, CSP) from "third party sends no CORS headers" (not fixable by the allowlist), rather than only a bare network error.
- A decision is recorded on whether to build the same-origin asset proxy (design option 3). If yes, it is filed as its own ticket with the SSRF/allowlist-enforcement requirements spelled out; if no, the limitation is recorded as a known constraint of URL ingest.
- Regression coverage for whichever path is chosen: if the proxy is built, a test asserts a proxied fetch is refused for an origin absent from the artifact's allowlist; if only diagnosis ships, a test or fixture pins the documented behavior of `<base href>` + residual origins for a runtime-fetched asset.


## Notes

**2026-08-11T18:58:06Z**

Verified workaround, and it changes the design picture: inlining the runtime-fetched asset as a `data:` URI WORKS today, with no code change.

Test (local instance, same ingested artifact): replaced the single call

    fetch('/build/wasm/pokeemerald.wasm', { cache: 'no-store' })

with `fetch('data:application/wasm;base64,<12222529 bytes -> 16296708 base64 chars>')` and PATCHed it via `PATCH /api/artifacts/:id {body}`. Result in Chrome: the artifact boots, status line reads "running - 11.7 MiB wasm", canvas begins painting. Previously it died at `wasmModule` with TypeError.

Why it works: the render CSP already carries `connect-src blob: data: ...` unconditionally (the no-egress bucket from av-x01o), so a `data:` fetch is permitted and, being same-document, has no CORS check at all.

Facts this establishes:

- `PATCH /api/artifacts/:id` accepted a 16.3 MB body with no size limit — there is no request-body cap on the write path. Worth knowing independently (possible DoS/quota concern; see av-4bzn for the adjacent unbounded-input theme).
- The render surface served the resulting 16.3 MB document fine, but with `Cache-Control: no-store` it re-transfers all 16.3 MB on every view. Acceptable on localhost, poor over a network.
- This artifact makes exactly ONE runtime network call - no Workers, no XHR, no other fetch - so a single inline fixed it completely. That will not generalize to artifacts with many or constructed URLs.

Implication for design option 2 (vendor runtime-fetched assets): the mechanism is proven, so the blocker is purely the size caps (`MaxAssetBytes` 5 MiB, `MaxTotalBytes` 20 MiB in fetcher.go:42-44) plus the base64 ~33% inflation and the no-store re-transfer cost. If option 2 is pursued, it needs a policy on large-asset budgets and probably a cacheable delivery path, not just a cap bump.

Also viable for users today without any exhibit change: re-host the asset on any origin that sends `Access-Control-Allow-Origin: *`, point the fetch at it, and allowlist that origin. Keeps the body small and the asset cacheable, at the cost of self-containment (and it re-introduces the live-linked dependency that PRD §9 rules out).
