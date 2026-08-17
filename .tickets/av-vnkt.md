---
id: av-vnkt
status: open
deps: []
links: [Exh-avau, av-1rvm, av-20fk, av-oz40]
created: 2026-08-17T03:19:40Z
type: feature
priority: 2
assignee: Max Omdal
tags: [export, portability, snapshot]
---
# Self-contained .html export — one file, no Exhibit dependency

PRD §7 and build-order step 3 promise a one-file self-contained `.html` export: the portable fallback for email/Slack/offline that needs no service at all. It has never been built. This ticket builds it, and fixes its scope so that it stays true when artifact assets stop living inside the artifact body.

The forcing case is large binary payloads. The snapshot vendorer's runtime pass (av-ghvs) currently base64s a wasm/`.data` payload straight into the document — up to 16 MiB fetched, ~21 MiB in the body (`internal/snapshot/fetcher.go:51`). That makes the body unreadable by the agent (`get_artifact` hands `a.body` verbatim to the model, `internal/agent/ext/exhibit.ts:209`), slow in CodeMirror, and re-transferred in full on every render because the render document is `Cache-Control: no-store`. The fix under consideration moves those payloads out of the body into their own blobs, addressed by URL.

That move is only acceptable if the artifact remains a portable file, which is architecture.md principle #1. This export is where that promise is kept.

## Design

**The invariant this ticket exists to enforce:**

> The out-of-line asset URL is an internal storage and transport representation. The **file** is the canonical artifact, and it is materialized at every boundary where the artifact leaves the service.

Concretely: one materialize function, called by every exit path, so no exit can forget. `GET /api/artifacts/:id/export` returns a single `text/html` document with every out-of-line asset folded back in as a `data:` URI — byte-for-byte the document the render surface would have served before assets were externalized. It depends on no origin, no token, and no running Exhibit instance.

**Why re-inlining is correct here and not everywhere.** A single file has nowhere else to put bytes, so `data:` is the only option and its 33% base64 tax is the price of the format. That is a one-off cost at export time, not the per-view cost the current design pays.

**The other exit paths, for contrast — do not unify them prematurely:**

- **Static build export ([[Exh-avau]] / [[Exh-imom]])** has a real filesystem, so it should emit `assets/<sha>.wasm` *beside* each artifact's HTML and rewrite the manifest to **relative** URLs. Self-contained as a directory, no Exhibit dependency, and it skips base64 entirely. Strictly better than inlining for that surface.
- **Shares (`/s/:id`)** stay server-mediated. A share was never durable past the service; this is not a regression. The asset route must accept share-scoped auth or shares break.
- **Account deletion** removes assets alongside the artifacts referencing them, so nothing is left half-broken. The requirement there is that deletion offer an export first — a property of [[av-4wyq]], not of this ticket.

**Interaction with the fetch wrapper.** The vendorer substitutes by interception, not source rewriting (`internal/snapshot/runtime.go`): a `window.fetch` wrapper consults a manifest keyed by absolute URL. Export therefore rewrites *manifest values*, not page source — the same seam either representation uses, so the exported file needs no transform the live document doesn't also apply.

**Content-addressing is per owner, not global.** Dedup within one library, never across libraries. Global dedup makes account deletion able to strip bytes out of another owner's artifact unless refcounting is exactly right in the delete path; per-owner addressing removes that failure mode by construction. Costs duplicate storage only on multi-user instances, and nothing on a single-user self-host.

**Scope note.** State is not exported by this ticket. An exported file carries the artifact's state inlined as of export time, read-only, matching the static build's accepted limitation; live write-through has no API to reach. State export as a first-class concern is [[av-1rvm]].

## Acceptance Criteria

- `GET /api/artifacts/:id/export` returns one `text/html` document, owner-scoped like every other artifact route.
- The exported file opens and runs correctly from `file://` with the Exhibit instance stopped, for an artifact whose assets are stored out of line — including a wasm artifact that instantiates its module.
- Every out-of-line asset appears in the exported file as a `data:` URI; the exported document contains no reference to `RENDER_ORIGIN`, no render token, and no path under `/a/<id>/assets/`. This is asserted by a test, not by inspection.
- A `.wasm` asset's `data:` URI carries `application/wasm`, so `WebAssembly.instantiateStreaming` still accepts it (the constraint `runtimeDataURI` already encodes).
- An artifact with no out-of-line assets exports unchanged — the export path is identity for the common case.
- The gallery offers the export from the artifact detail page, and the filename derives from the artifact title.
- Docs state the invariant (URL form is internal; the file is canonical, materialized at every exit) and name each exit path and which representation it uses.

