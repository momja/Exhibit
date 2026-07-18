---
id: av-xkq4
status: open
deps: []
links: [Exh-yvhp]
created: 2026-07-16T05:33:05Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent, render, build]
---
# Replace agent snippet tool with BOWSER_SNAPS SDK (drag-select region snapper)

## Context

The agent "snippet mode" (Exh-edjk) lets a user point at part of the live artifact preview and attach it to the next agent prompt as multimodal context. Today it is a **bespoke inline element-picker** in `internal/render/snippet.go`: the app-origin host activates it via `postMessage`, the user **clicks one element**, and it captures a hand-rolled structural descriptor + a single-element SVG-`foreignObject` screenshot, posting `{descriptor, image}` to the host (`internal/api/agentui.go`).

Replace that hand-rolled tool with the owner's **BOWSER_SNAPS SDK** (https://github.com/momja/BOWSER_SNAPS, `sdk/` — pure dependency-free ESM). The SDK does the same job far better: a macOS-⌘⇧4-style **drag-select region** overlay plus a rich metadata collector — every element in the region with stable selectors, React/Vue/Angular **component names**, detected **frameworks**, a sanitized **DOM snippet**, and a rolling **console-error** buffer. That is strictly more context for "make this thing green / this area is broken" prompts, and it retires ~200 lines of bespoke picker/rasterizer code in favor of a maintained SDK the owner controls.

**Scope decisions (from the owner):**
- **Drag-select region replaces all interaction for now.** The SDK will soon add click-to-pick; when it does it should slot in "roughly for free" behind the same host protocol — so keep the host/render message contract generic (`activate`/`deactivate`/`captured`/`cancelled`), not region-specific.
- **Vendor the SDK the same way as CodeMirror/Phosphor:** an npm dependency in a new `web/` build workspace, esbuild-bundled at build time (Node is build-time-only; output is gitignored, not committed). The only twist: `bowser-snaps` is not on the public registry, so the dependency is `github:momja/BOWSER_SNAPS` (npm resolves git deps fine; `--private` only blocks publish, not install).

**Security model is unchanged and must stay that way.** Capture still happens *inside* the sandboxed, opaque-origin artifact iframe (the host cannot reach into it; the SDK's component detection needs to run in the artifact's own JS world anyway). The picker stays inert until the **app-origin** host activates it (origin-checked), all posts are pinned to `APP_ORIGIN`, and top-level/share renders (no parent frame) stay inert. The render doc's CSP is already `script-src 'unsafe-inline' 'unsafe-eval'`, so the bundled IIFE injects inline exactly like the current script — no CSP change. `getDisplayMedia` is unavailable in the opaque-origin sandbox, so we keep supplying **our own pixels** (the existing `foreignObject`→canvas rasterization, generalized from one element to the selection region) to the SDK's bring-your-own-pixels `capture` hook.

## Design

Cohesive single-branch feature. Five parts, in order.

### 1. New asset workspace `web/snippet/` (mirror `web/editor`)

Per `docs/build_assets.md`: any `web/*/package.json` with a `build` script is auto-discovered by `scripts/build-assets.sh` — no Dockerfile/Makefile edits needed.

- `web/snippet/package.json`: `description` documenting what it builds and where; dependency `"bowser-snaps": "github:momja/BOWSER_SNAPS"`; devDependency `esbuild` (match `web/editor`'s version). `build` script:
  `esbuild snippet-entry.js --bundle --minify --format=iife --target=es2020 --outfile=../../internal/api/assets/snippet.js`
  Output lands under the already-embedded `internal/api/assets/` root (see `internal/api/assets.go` `//go:embed assets`). Commit the generated `package-lock.json`; `.gitignore` already excludes `internal/api/assets/` and `node_modules/`.
- `web/snippet/snippet-entry.js` — the glue that wires the SDK to the host protocol. Imports from `bowser-snaps`:
  `import { selectRegion, collectRegionMetadata, createErrorMonitor, buildPageContext, deviceRect, cropToPng } from 'bowser-snaps';`
  Behavior:
  - **On load (runs first in `<head>`, before artifact scripts):** `const errorMonitor = createErrorMonitor();` so console errors are buffered for any later snap. Read the host origin from a global the render prelude sets: `const APP_ORIGIN = window.__EXHIBIT_APP_ORIGIN;`. If there is no parent frame (`window.parent === window`), do nothing further (inert on direct/share renders).
  - **Message listener** (`window.addEventListener('message', …)`): ignore anything whose `e.origin !== APP_ORIGIN` or without `e.data.__exSnippet`. Handle `'activate'` / `'deactivate'`. This is the same generic contract the current script uses, so future click-to-pick reuses it.
  - **On `activate`:** run `const rect = await selectRegion();`. `null` (Esc / sub-4px) → post `{__exSnippet:'cancelled'}`. Otherwise: `const metadata = collectRegionMetadata(rect);` then rasterize a **BYO-pixels** capture and crop:
    - `capture({rect, viewport})`: serialize a bounded clone of `document.documentElement` (reuse the current file's node-cap ≈300 / max-px ≈2000 guards and inline-computed-styles trick) into an SVG `foreignObject` sized to the viewport, draw to a canvas, `createImageBitmap` it. On any failure return `null`.
    - If capture succeeded: `const dr = deviceRect(rect, viewport, bitmap.width, bitmap.height); const pngBytes = await cropToPng(bitmap, dr);` then base64-encode to `{ data, mimeType:'image/png' }`. If capture failed, proceed **image-less** (metadata-only) — same graceful degradation as today.
    - Enrich the posted metadata with `page` (`buildPageContext()`) and `consoleErrors` (`errorMonitor.snapshot()`) alongside the collector's `{elements, frameworks, domSnippet}`. (Equivalent shortcut: build the whole thing via the SDK's higher-level `createSnapper({capture, errorMonitor})` and post `snap.metadata`, skipping its PNG-embed step which the agent path doesn't need. Prefer whichever reads cleaner — the building blocks avoid pulling in `embedMetadata`.)
    - Post `{__exSnippet:'captured', metadata, image}` to `APP_ORIGIN`. Do **not** send the old single-element `descriptor` field.
  - **Esc while active** → deactivate + `{__exSnippet:'cancelled'}`.
  - Keep the overlay/teardown entirely inside the SDK (`selectRegion` owns its own shadow-DOM overlay), so the render script shrinks to glue.

### 2. Wire the bundle into the render surface as injected data (keep `render` Node-free)

Do **not** add a second `//go:embed` root in `internal/render` — that would make `go test ./internal/render` require the Node asset build. Instead inject the built bundle as data from the composition root that already embeds assets:

- `internal/api/api.go` (~line 134, `render.New(render.Config{…})`): read the embedded `assets/snippet.js` (via the existing api assets FS) and pass it in a new `render.Config.SnippetBundle string` field.
- `internal/render/render.go`: add `SnippetBundle string` to `Config`. Thread it into `injectShim` (add a `snippetBundle string` param; `serveArtifactDoc` passes `rd.cfg.SnippetBundle`).
- `internal/render/snippet.go`: `snippetScript(appOrigin, bundle string)` returns a small **origin prelude** `<script>window.__EXHIBIT_APP_ORIGIN=%q</script>` (origin `%q`-quoted, the one dynamic value) immediately followed by `<script>` + `bundle` + `</script>`. Delete the giant hand-rolled `snippetTemplate` constant and its element-picker/rasterizer body. Keep the doc comment explaining the inert-until-activated, origin-pinned contract.
- Existing `injectShim` tests (`internal/render/render_test.go`, the `injectShim("<head></head>", "abc", "https://app.test", nil)` calls) get an extra arg — pass a small stub bundle string (e.g. `"/*snip*/"`); they assert on shim content, not snippet content, so they keep passing with **no Node dependency**.

### 3. Host consumer `internal/api/agentui.go`

The received-message handler and prompt formatter move from a single-element `descriptor` to the SDK `metadata` object.

- `pendingSnippets` entries become `{image, metadata, thumbUrl}` (drop `descriptor`). Update the init comment (~line 202) and the `captured` handler (~line 590): read `d.metadata` (+ `d.image`); build `thumbUrl` from `d.image` as today.
- `renderSnippetChips` (~line 606): keep the thumbnail; label the chip from metadata — e.g. the first element's `selector`, or `"N elements"` when several (`s.metadata.elements && s.metadata.elements[0] && s.metadata.elements[0].selector`).
- Replace `describeSnippet(descriptor)` (~line 522) with `describeSnapMetadata(metadata)` producing a compact agent-readable block: page path, `frameworks`, an elements list (selector · tag · component name · trimmed text), `consoleErrors` (if any — this is often the bug), and the trimmed `domSnippet`. This is the main UX win; keep it terse (the region can carry up to ~40 elements — cap the rendered list, e.g. top ~8 in the SDK's smallest-area-first order).
- `send()` (~line 493): `images` mapping is unchanged (one PNG per snap → `{data, mime_type}`); message text uses `describeSnapMetadata`.

### 4. Agent system prompt `internal/agent/agent.go`

Update the snippet paragraph (~line 115) from "an attached screenshot plus an element descriptor with selector/outerHTML" to reflect the region snap: a screenshot of a selected region plus metadata listing the elements in it (selectors, component names), detected frameworks, console errors, and a DOM snippet — instruct the agent to locate the intended element(s) via those selectors/component names and to treat console errors as likely-relevant.

### 5. Docs + tests

- **Docs (same PR — repo rule: docs describe only what's built):**
  - `docs/agent.md` "Snippet mode" section — rewrite for the SDK region snapper (what it captures now, the BYO-pixels capture, the unchanged security posture). Note click-to-pick is a future SDK addition behind the same host contract.
  - `docs/architecture.md` §3.2 (render surface) — the snippet picker is now the BOWSER_SNAPS SDK bundle injected inline; §3.5/§13 asset-workspace mentions gain `web/snippet`.
  - `docs/technical_stack.md` §1 stack table — add the snapshot SDK / `web/snippet` esbuild workspace row.
  - `docs/build_assets.md` — `web/snippet` is auto-discovered; note it outputs `internal/api/assets/snippet.js` and is injected into the render surface via `render.Config.SnippetBundle` (not served to browsers directly).
- **Tests:**
  - `internal/render/render_test.go` — update `injectShim` call sites for the new param; add a focused test that `snippetScript` emits the origin prelude (`__EXHIBIT_APP_ORIGIN` `%q`-quoted) followed by the injected bundle, and that the origin is escaped safe for the `<script>` context.
  - `internal/api` agentui tests — a unit test for `describeSnapMetadata` over a representative SDK metadata object (elements + frameworks + a console error) asserting the compact text shape.
  - The mock-LLM E2E path (`cmd/mockllm`, which already scripts snippet acknowledgment) still exercises the pipeline; verify it drives a change end-to-end with the new payload (the `verify` skill / `docs/agent.md` §Configuration harness).

### Rejected alternative

`//go:embed assets/snippet.js` directly in `internal/render` — simpler wiring, but couples `go test ./internal/render` to the Node asset build. Injecting the bundle as `render.Config.SnippetBundle` from `internal/api` (which already embeds `assets`) keeps `render` a pure transformer and its tests Node-free (Ousterhout: pull complexity down / treat the asset as data).

## Acceptance Criteria

- `web/snippet/` is a discovered asset workspace: `make assets` (and `docker build`) bundle `bowser-snaps` (github dep) via esbuild into `internal/api/assets/snippet.js`; nothing generated is committed; a checkout that skips the asset build fails loud (empty embed dir), as with the other workspaces.
- In the agent preview (`/agent?artifact=<id>`), activating snippet mode (Snip button / Ctrl+Shift+S) shows the SDK's drag-select overlay; dragging a region attaches a chip with a cropped region screenshot; the next prompt carries that screenshot as a multimodal image plus a compact metadata block (elements with selectors + component names, frameworks, console errors, DOM snippet). Esc cancels cleanly.
- The bespoke element-picker/rasterizer in `internal/render/snippet.go` is gone; the injected script is the SDK IIFE bundle + an origin prelude, injected inline inside the sandbox, inert until the app-origin host activates it, all posts pinned to `APP_ORIGIN`, and inert on direct/share (top-level) renders. No CSP change.
- Security posture unchanged: capture stays inside the opaque-origin iframe; origin checks on activation and on every post are intact.
- `render` stays Node-free to test: the bundle arrives via `render.Config.SnippetBundle`; `go test ./internal/render` passes without running the Node build.
- `go build ./...` + `go test ./...` (after `make assets`) pass; `web/snippet` builds clean; the mock-LLM flow drives an artifact edit via a region snap end-to-end.
- Docs updated in the same PR (`agent.md`, `architecture.md`, `technical_stack.md`, `build_assets.md`); the future click-to-pick addition is noted as riding the same host contract.

## Notes

- Reuse over rebuild: the SDK already ships the overlay (`selectRegion`), collectors (`collectRegionMetadata`, `detectFrameworks`), error buffer (`createErrorMonitor`), page context (`buildPageContext`), and cropping (`deviceRect`/`cropToPng`). The only bespoke code to keep is the sandbox rasterization for the BYO-pixels `capture` (generalized from the current per-element `foreignObject` rasterizer) — the SDK's default `captureViaDisplayMedia` won't work in the opaque-origin sandbox.
- Branch per project rules: `feature/av-xkq4/bowser-snaps-snippet` via a supacode worktree; never develop on `main`.
