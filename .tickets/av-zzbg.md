---
id: av-zzbg
status: open
deps: [av-c23y]
links: []
created: 2026-07-22T03:17:44Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-xkq4
tags: [api, gallery, snippet]
---
# Reusable host-side capture client (extract from agentui.go into a shared, agent-agnostic module)

Parent epic: [[av-xkq4]]. Depends on [[av-c23y]] (consumes the capture protocol it
defines). This is the **decoupling** story — the one that actually makes region capture
independent of the agent. It produces no user-visible feature on its own; its output is a
reusable boundary that [[av-g2f3]] then consumes.

## Problem

Today the entire host side of capture is hand-inlined into the agent chat page's inline
JS string (`internal/api/agentui.go`, `renderAgentPage`): `pendingSnippets`,
`toggleSnippet`, the `captured` message handler, `renderSnippetChips`, `clearSnippets`,
and the activation keybinding all live there, tangled with agent-only concerns
(`describeSnippet`, `send()`, session state). There is no reusable capture client, so a
second consumer would have to copy it. Extract the agent-agnostic parts into a shared
module first, so the agent (and any future consumer) merely *mounts and subscribes*.

## Design

### 1. Create the shared module

Follow the gallery's static-asset convention (`web/gallery` workspace, served under
`/assets/gallery/`, `go:embed`-ed) rather than another inline Go string. A small vanilla
module — no framework — e.g. `web/gallery/capture-client.js` (or a dedicated
`web/capture` workspace if the build owner prefers a separate bundle; match whatever
`docs/build_assets.md` makes cheapest). Public surface, roughly:

```
const client = createCaptureClient(iframeEl, {
  appOrigin,                 // pins postMessage + validates inbound origin
  chipContainer,             // optional: element the client renders chips into
  onCaptured(capture) {},    // {metadata, image} — consumer decides what to do
  onCancelled() {},
});
client.activate();           // posts {__exSnippet:'activate'} to the iframe
client.deactivate();
client.toggle();
client.pending();            // current captures (for consumers that batch, e.g. agent)
client.clear();
```

Responsibilities (all agent-agnostic):
- Post `activate`/`deactivate` to the iframe (protocol from [[av-c23y]]), pinned to
  `appOrigin`.
- Listen for `{__exSnippet:'captured'|'cancelled'}`, validating `e.source === iframeEl.contentWindow`
  **and** `e.origin` per the protocol; build the thumbnail from `image`; maintain the
  pending list; render/clear chips (the chip UI moves here verbatim from `agentui.go`,
  including the Phosphor `ph-x` remove button — icons stay self-hosted).
- Emit `onCaptured({metadata, image})` — the client does **not** know what `metadata`
  means to a consumer; it just hands it over. **No prompt text, no `send()`, no
  `describeSnapMetadata`** here — those are agent-specific and belong to [[av-g2f3]].

### 2. Do not leak agent concepts

Hard boundary for reviewability of the epic's modularity goal:
- The module must not reference sessions, prompts, the agent API, or `describeSnippet`.
- Keybinding policy (Ctrl+Shift+S, Esc) can be an *opt-in helper* the client exposes
  (e.g. `client.bindKeys(document)`), but the consumer chooses to call it — the client
  doesn't assume it owns global keys.
- The chip container is injected by the consumer, so a non-agent page can place chips
  wherever (or pass none and render its own from `onCaptured`).

### 3. Serve/embed it

Wire the new asset into the build + embed like the other gallery assets
(`docs/build_assets.md`), and expose it to pages that need it (the agent page loads it in
[[av-g2f3]]). Keep it out of the render origin — it is host/app-origin code.

## Acceptance Criteria

- A shared, framework-free capture-client module exists, built/embedded via the gallery
  asset pipeline (not an inline Go string), with the public surface above.
- The module contains **zero** agent/session/prompt references — grep-clean of
  `describeSnippet`, `send`, `session`, `prompt`. Its only external contract is the
  [[av-c23y]] capture protocol + the `appOrigin` it's given.
- It is demonstrably mountable by a non-agent page: include a tiny harness or a unit/DOM
  test that mounts it against a stub iframe, simulates a `captured` postMessage from the
  correct source/origin, and asserts `onCaptured` fires and a chip renders; and that a
  wrong-origin/wrong-source message is ignored.
- Origin/source validation on inbound messages and origin-pinning on outbound posts are
  intact (security posture unchanged from the epic).
- `agentui.go` is **not** yet rewired in this story (that's [[av-g2f3]]); this story may
  leave the old inline handling in place or behind, but the new module must stand alone.
  (If landing A→B→C on one merge branch, B and C can be sequential commits.)

## Notes

- Branch per project rules via a supacode worktree, e.g.
  `feature/av-zzbg/capture-client`; never develop on `main`.
- This story is the concrete answer to the epic's goal "interaction/consumers vary
  independently": after it, adding a new *consumer* means calling `createCaptureClient`;
  adding a new *interaction pattern* is entirely inside [[av-c23y]] and this client
  never changes.
