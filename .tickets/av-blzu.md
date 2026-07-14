---
id: av-blzu
status: open
deps: []
links: []
created: 2026-07-08T07:06:01Z
type: feature
priority: 3
assignee: Max Omdal
parent: av-7k7b
tags: [ui, share, state, render]
---
# Make direct (non-iframe) artifact open an explicit static/read-only snapshot view

## Acceptance Criteria

- The artifact detail page's 'Open in new tab ↗' affordance communicates that opening an artifact directly (top-level, not embedded) is a static/snapshot view, not a fullscreen interactive session: state reads use the last persisted snapshot and writes made in that session are NOT saved back to the server.
- A visitor who lands on a top-level rendered artifact (/a/:id opened outside the app's embedding host) is made aware that persistence is inactive for this view (label/tooltip on the action, and/or a runtime banner the shim injects when window.parent === window).
- Docs (architecture.md §6 / technical_stack.md) state explicitly that the shim's write path is a no-op outside the embedded host frame, so top-level serve is a static/share view rather than a fullscreen run.
- The direct-open 'static view' and the share '/s/:id' 'static view' are presented under one coherent product concept (a static, non-persisting view), even though they differ in shim presence (owner direct-open keeps the shim for read continuity; shares omit it for privacy per av-7k7b).
- No regression to the embedded viewer: writes still persist when the artifact runs in the detail page's sandboxed iframe with the host frame present.

## Design notes / mechanism

The shim is injected unconditionally by the render surface for every served
`/a/:id` document (`internal/render/render.go: serveArtifactDoc` →
`injectShim`), so `top-level direct open gets the same shim + inlined state as
the embedded iframe. Reads work in both contexts (synchronous from the inlined
`cache`). The write path differs and is the crux of this story:

- The shim's `setItem`/`removeItem` update the in-memory `cache` *and* call
  `writeThrough`, which posts to `window.parent`. The guard
  `if (window.parent === window) return; // top-level: no host to persist through`
  (in `shimTemplate`, internal/render/render.go) causes direct-open to update
  only the session cache — the `PUT /api/artifacts/:id/state` never fires
  because there is no authenticated host frame on the app origin to bridge it.
  Reload = lost writes. There is no error, no warning.
- The sandbox's opaque origin also means the doc can't call the API itself
  (why `connect-src` need not include the app origin — see architecture §6),
  so the host frame is the *only* persistence channel; without it the view is
  read-only by construction.

Three known load paths today:
1. Embedded in the app detail page iframe → reads inlined, writes persist (host).
2. Owner 'Open in new tab ↗' (top-level `/a/:id`) → shim present, reads inlined,
   writes session-only, NOT persisted (silent loss — the footgun this story fixes).
3. Share `/s/:id` (top-level, per `av-7k7b`) → no shim at all (privacy),
   `localStorage` throws SecurityError in the opaque sandbox; already documented
   on the share UI.

The framing `av-7k7b` adopted for path 3 ("a static, non-persisting view") is
the right model for path 2 as well: direct open is more 'share static' than
'fullscreen interactive'. The story is to make that honest — relabel/repurpose
the `Open in new tab` affordance and/or have the shim emit a runtime banner when
it detects there is no host to write through. A runtime banner would be a small
addition to `shimTemplate` guarded on `window.parent === window`. Note the
share path (no shim) needs its own equivalent surface, since it can't reuse the
shim's banner — keep the two consistent in product copy.

