---
id: av-c23y
status: open
deps: []
links: []
created: 2026-07-22T03:17:44Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-xkq4
tags: [render, build, snippet]
---
# Region-capture producer: web/snippet (BOWSER_SNAPS drag-select) + render injection + formal protocol

Parent epic: [[av-xkq4]] (read it for the isolation analysis and the cross-cutting
security/vendoring/Node-free constraints — they all apply here and are not repeated in
full). This is the **producer** end: the in-sandbox capture module and the contract it
speaks. It has **no knowledge of the agent**; consumers ([[av-zzbg]], [[av-g2f3]]) build
on the protocol this story defines.

## Scope

Replace the bespoke element-picker in `internal/render/snippet.go` with a BOWSER_SNAPS
drag-select region snapper bundled from a new `web/snippet` workspace, injected into the
render surface as data, and speaking a **formalized capture protocol**.

## Design

### 1. New asset workspace `web/snippet/` (mirror `web/editor`)

Per `docs/build_assets.md`, any `web/*/package.json` with a `build` script is
auto-discovered by `scripts/build-assets.sh` — no Dockerfile/Makefile edits.

- `web/snippet/package.json`: `private: true`; a `description` documenting what it builds
  and where; dependency `"bowser-snaps": "github:momja/BOWSER_SNAPS"`; devDependency
  `esbuild` (match `web/editor`'s `^0.25.x`). `build` script:
  `esbuild snippet-entry.js --bundle --minify --format=iife --target=es2020 --outfile=../../internal/api/assets/snippet.js`
  Output lands under the already-embedded `internal/api/assets/` root
  (`internal/api/assets.go` `//go:embed assets`). Commit `package-lock.json`;
  `.gitignore` already excludes `internal/api/assets/` and `node_modules/`.
- `web/snippet/snippet-entry.js` — the glue wiring the SDK to the protocol. Import:
  `import { selectRegion, collectRegionMetadata, createErrorMonitor, buildPageContext, deviceRect, cropToPng } from 'bowser-snaps';`
  Behavior:
  - **On load (runs first in `<head>`, before artifact scripts):**
    `const errorMonitor = createErrorMonitor();` so console errors are buffered for any
    later snap. Read the host origin from a global the render prelude sets:
    `const APP_ORIGIN = window.__EXHIBIT_APP_ORIGIN;`. If `window.parent === window`
    (no host frame) do nothing further — inert on direct/share renders.
  - **Message listener:** ignore anything with `e.origin !== APP_ORIGIN` or without
    `e.data.__exSnippet`; handle `'activate'`/`'deactivate'`.
  - **On `activate`:** `const rect = await selectRegion();`. `null` (Esc / sub-4px) →
    post `{__exSnippet:'cancelled'}`. Otherwise `const metadata = collectRegionMetadata(rect);`
    then a **BYO-pixels** capture + crop:
    - `capture({rect, viewport})`: serialize a bounded clone of `document.documentElement`
      (reuse the current file's node-cap ≈300 / max-px ≈2000 guards and inline-computed-
      styles trick) into an SVG `foreignObject` sized to the viewport, draw to a canvas,
      `createImageBitmap` it. On any failure return `null`.
    - If captured: `const dr = deviceRect(rect, viewport, bitmap.width, bitmap.height);`
      `const pngBytes = await cropToPng(bitmap, dr);` → base64 → `{data, mimeType:'image/png'}`.
      If capture failed, proceed **image-less** (metadata-only) — same graceful
      degradation as today.
    - Enrich posted metadata with `page` (`buildPageContext()`) and `consoleErrors`
      (`errorMonitor.snapshot()`) alongside the collector's `{elements, frameworks,
      domSnippet}`. (Equivalent shortcut: `createSnapper({capture, errorMonitor})` and
      post `snap.metadata`, skipping its PNG-embed step the agent path doesn't need —
      prefer whichever reads cleaner; the building blocks avoid pulling in
      `embedMetadata`.)
    - Post `{__exSnippet:'captured', metadata, image}` to `APP_ORIGIN`. Do **not** send
      the old single-element `descriptor` field.
  - **Esc while active** → deactivate + `{__exSnippet:'cancelled'}`.
  - Overlay/teardown stays entirely inside the SDK (`selectRegion` owns its own
    closed-shadow-DOM overlay), so this file stays glue.

### 2. Formalize the capture protocol (the seam)

This is the modularity linchpin — make the contract explicit so interaction patterns
(this story) and consumers ([[av-zzbg]]/[[av-g2f3]]) evolve independently.

- Document it in `docs/agent.md` (or a short `docs/` note the epic references): the
  message names, direction, origin/frame rules, and payload shapes:
  - **Host → sandbox:** `{__exSnippet:'activate'}` / `{__exSnippet:'deactivate'}`
    (origin-checked against `APP_ORIGIN`).
  - **Sandbox → host:** `{__exSnippet:'captured', metadata, image}` (`image` may be
    `null`) / `{__exSnippet:'cancelled'}` (posted to `APP_ORIGIN`).
  - `metadata` shape: `{page, frameworks, elements[], domSnippet, consoleErrors}` as
    produced by the SDK — document the fields consumers may rely on.
- Keep the message key generic (`__exSnippet`, or rename to `__exCapture` if preferred —
  decide once here since both consumers key on it). Interaction is **not** encoded in the
  message: `activate` means "begin capture with whatever the current interaction is,"
  so click-to-pick later needs no new message.

### 3. Wire the bundle into render as injected data (keep `render` Node-free)

Do **not** add a `//go:embed` root in `internal/render` (it would make
`go test ./internal/render` require the Node build). Inject from the composition root
that already embeds assets:

- `internal/api/api.go` (~line 134, `render.New(render.Config{…})`): read the embedded
  `assets/snippet.js` (existing api assets FS) and pass a new
  `render.Config.SnippetBundle string`.
- `internal/render/render.go`: add `SnippetBundle string` to `Config`; thread into
  `injectShim` (new `snippetBundle string` param; `serveArtifactDoc` passes
  `rd.cfg.SnippetBundle`).
- `internal/render/snippet.go`: `snippetScript(appOrigin, bundle string)` returns a
  small **origin prelude** `<script>window.__EXHIBIT_APP_ORIGIN=%q</script>` (origin
  `%q`-quoted — the one dynamic value) followed by `<script>` + `bundle` + `</script>`.
  Delete the giant `snippetTemplate` constant and its picker/rasterizer body; keep the
  doc comment explaining the inert-until-activated, origin-pinned contract.

### 4. Tests (render package — no Node dependency)

- Update existing `injectShim(...)` call sites in `internal/render/render_test.go` for
  the new param; pass a small stub bundle (e.g. `"/*snip*/"`). They assert shim content,
  not snippet content, so they keep passing without the Node build.
- Add a focused test that `snippetScript` emits the `__EXHIBIT_APP_ORIGIN` origin prelude
  (`%q`-quoted, escaped safe for the `<script>` context) followed by the injected bundle.

### 5. Docs (this PR)

- `docs/architecture.md` §3.2 — the render surface's injected picker is now the
  BOWSER_SNAPS bundle; §3.5/§13 asset-workspace mentions gain `web/snippet`.
- `docs/technical_stack.md` §1 stack table — add the snapshot SDK / `web/snippet`
  esbuild workspace row.
- `docs/build_assets.md` — `web/snippet` is auto-discovered; outputs
  `internal/api/assets/snippet.js`, injected into render via `render.Config.SnippetBundle`
  (not served to browsers directly).
- The capture protocol write-up from Design §2.

## Acceptance Criteria

- `web/snippet/` is a discovered asset workspace: `make assets` (and `docker build`)
  bundle `bowser-snaps` (github dep) via esbuild into `internal/api/assets/snippet.js`;
  nothing generated is committed; a checkout skipping the asset build fails loud (empty
  embed dir), as with the other workspaces.
- The bespoke picker/rasterizer in `internal/render/snippet.go` is gone; the injected
  script is the SDK IIFE bundle + origin prelude, inline in the sandbox, inert until the
  app-origin host activates it, all posts pinned to `APP_ORIGIN`, inert on
  direct/share (top-level) renders. No CSP change.
- Activating in a sandboxed iframe shows the SDK's drag-select overlay; a region posts
  `{__exSnippet:'captured', metadata, image}`; Esc/sub-4px posts `cancelled`.
- The capture protocol is documented (message names, direction, origin/frame rules,
  metadata shape) and is interaction-agnostic (click-to-pick would need no new message).
- `render` stays Node-free to test: bundle arrives via `render.Config.SnippetBundle`;
  `go test ./internal/render` passes without the Node build.

## Notes

- Branch per project rules via a supacode worktree, e.g.
  `feature/av-c23y/snippet-producer`; never develop on `main`.
- The SDK's default `captureViaDisplayMedia` won't work in the opaque-origin sandbox —
  keep the BYO-pixels `foreignObject`→canvas rasterization (generalized from per-element
  to region).
