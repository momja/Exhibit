---
id: av-xkq4
status: open
deps: []
links: [Exh-yvhp]
created: 2026-07-16T05:33:05Z
type: epic
priority: 2
assignee: Max Omdal
tags: [agent, render, build, snippet]
---
# Epic: Modular region capture — BOWSER_SNAPS SDK, decoupled from the agent

## Context

The agent "snippet mode" (Exh-edjk) lets a user point at part of the live artifact
preview and attach it to the next agent prompt as multimodal context. Today it is a
**bespoke inline element-picker** in `internal/render/snippet.go`: the app-origin host
activates it via `postMessage`, the user **clicks one element**, and it captures a
hand-rolled structural descriptor + a single-element SVG-`foreignObject` screenshot,
posting `{descriptor, image}` to the host (`internal/api/agentui.go`).

Two goals for this epic:

1. **Replace** the hand-rolled picker with the owner's **BOWSER_SNAPS SDK**
   (https://github.com/momja/BOWSER_SNAPS, `sdk/` — pure dependency-free ESM): a
   macOS-⌘⇧4-style **drag-select region** overlay plus a rich metadata collector
   (every element in the region with stable selectors, React/Vue/Angular **component
   names**, detected **frameworks**, a sanitized **DOM snippet**, and a rolling
   **console-error** buffer). Strictly more context for "make this area green / this is
   broken" prompts, and it retires ~200 lines of bespoke picker/rasterizer code in
   favour of a maintained SDK the owner controls.
2. **Make the capture mechanism modular and independent of the agent**, so interaction
   patterns can be added or swapped without touching any consumer.

### Why an epic (the isolation analysis that drove the split)

The current feature has two halves, isolated very differently:

- **Capture side (`internal/render/snippet.go`) — already decoupled from the agent.**
  Zero Go deps but `fmt`; wired in through exactly **one** call site
  (`render.go:607`, inside `injectShim`). It does not know the agent exists — it picks,
  builds `{descriptor, image}`, and `postMessage`s to `APP_ORIGIN`. Swapping the
  interaction pattern is a change wholly within this file + its build.
- **Consumer side (`internal/api/agentui.go`) — welded to the agent, not modular.**
  The result is consumed in exactly one place: the agent chat page's **inline** JS
  string (`renderAgentPage`). `pendingSnippets`, `toggleSnippet`, the `captured`
  handler, `describeSnippet`, `renderSnippetChips`, `clearSnippets`, and `send()` are
  all hand-inlined there. No reusable capture client exists — any non-agent consumer
  would have to copy that machinery.
- **The seam between them (`__exSnippet` postMessage) is informal.** String-keyed
  `activate`/`deactivate`/`captured`/`cancelled` messages with an implicit payload
  shape, matched by convention on both ends, formalized nowhere.

So the interaction side is already agent-independent; what is *not* independent is that
the only consumer is the agent, welded into `agentui.go`, over an informal protocol.
True modularity means (a) formalizing that seam and (b) giving the consumer side a
reusable boundary. That is more than one cohesive PR — hence the decomposition below.

## Decomposition

```
av-c23y  Producer  ──►  av-zzbg  Capture client  ──►  av-g2f3  Agent consumer
(render + web/snippet    (extract reusable, agent-      (rewire agentui.go +
 + formal protocol)       agnostic host module)          prompt + docs + E2E)
```

- **[[av-c23y]] — Producer.** New `web/snippet` esbuild workspace wrapping
  BOWSER_SNAPS (drag-select region snap); inject the built bundle into the render
  surface as `render.Config.SnippetBundle` data; delete the bespoke picker/rasterizer.
  **Owns and documents the formal capture protocol** (`activate`/`deactivate` in;
  `captured{metadata,image}`/`cancelled` out) so interaction patterns and consumers
  vary independently. Emits the region snap; knows nothing about the agent. This is the
  home for future interaction patterns (click-to-pick slots in here behind the same
  contract).
- **[[av-zzbg]] — Reusable capture client.** Extract the host-side capture handling out
  of `agentui.go`'s inline script into a small, **agent-agnostic** shared module
  (`web/gallery` asset convention): `mount(iframe)` → activate/deactivate + emit a
  `captured` event to a subscriber; owns the chip/thumbnail UI. No agent, no prompt
  logic. Acceptance requires it be mountable by a non-agent page. Depends on av-c23y
  (consumes its protocol).
- **[[av-g2f3]] — Agent as a thin consumer.** Rewire `agentui.go` to *use* the capture
  client, add `describeSnapMetadata` (the agent-specific presentation of the SDK
  metadata), update the `agent.go` system prompt, and the docs + mock-LLM E2E. Depends
  on av-zzbg.

Stories are separately reviewable; A→B→C is a hard dependency chain but they may land
on one `feature/av-xkq4/*` merge branch rather than `main` if worked in sequence.

## Cross-cutting constraints (apply to every child)

- **Security model unchanged.** Capture stays *inside* the sandboxed, opaque-origin
  artifact iframe (the host can't reach in; the SDK's component detection needs the
  artifact's own JS world). The picker stays inert until the **app-origin** host
  activates it (origin-checked), all posts are pinned to `APP_ORIGIN`, and
  top-level/share renders (no parent frame) stay inert. Render CSP is already
  `script-src 'unsafe-inline' 'unsafe-eval'` — the bundled IIFE injects inline exactly
  like today; **no CSP change**. `getDisplayMedia` is unavailable in the opaque-origin
  sandbox, so we keep supplying **our own pixels** (the existing `foreignObject`→canvas
  rasterization, generalized from one element to the region) to the SDK's
  bring-your-own-pixels `capture` hook.
- **Vendoring like CodeMirror/Phosphor.** npm dependency in a `web/*` esbuild
  workspace, build-time only, output gitignored under `internal/api/assets/`. The one
  twist: `bowser-snaps` is not on the public registry, so the dep is
  `github:momja/BOWSER_SNAPS` (npm resolves git deps fine; `--private` only blocks
  publish, not install).
- **`render` stays Node-free to test.** The bundle reaches `render` as injected data
  (`render.Config.SnippetBundle`), never a second `//go:embed` in `internal/render`, so
  `go test ./internal/render` needs no asset build. (Ousterhout: treat the asset as
  data; pull complexity down.)
- **Docs describe only what's built** — each child updates docs *in its own PR*, not
  ahead of the code.

## Epic-level acceptance

- The bespoke element-picker/rasterizer in `internal/render/snippet.go` is gone;
  region capture is the BOWSER_SNAPS SDK bundle, injected inline in the sandbox, inert
  until app-origin activation, all posts pinned to `APP_ORIGIN`, inert on
  direct/share renders. No CSP change.
- A **documented, versioned capture protocol** exists (av-c23y) and is the only contract
  between producer and consumers.
- A **reusable, agent-agnostic capture client** exists (av-zzbg) — the agent is one
  consumer of it, not the owner of the capture UI. Adding a new interaction pattern
  (e.g. click-to-pick) touches the producer only; adding a new consumer touches neither
  producer nor client.
- In the agent preview (`/agent?artifact=<id>`), drag-selecting a region attaches a
  cropped screenshot chip; the next prompt carries that image plus a compact metadata
  block (elements + selectors + component names, frameworks, console errors, DOM
  snippet). Esc cancels cleanly.
- `go build ./...` + `go test ./...` (after `make assets`) pass; `web/snippet` builds
  clean; the mock-LLM flow drives an artifact edit via a region snap end-to-end.

## Notes

- Reuse over rebuild: the SDK already ships the overlay (`selectRegion`), collectors
  (`collectRegionMetadata`, `detectFrameworks`), error buffer (`createErrorMonitor`),
  page context (`buildPageContext`), and cropping (`deviceRect`/`cropToPng`). The only
  bespoke code to keep is the sandbox rasterization for the BYO-pixels `capture`.
- Full per-story implementation detail lives in the child tickets; this epic is the map
  and the shared constraints.
