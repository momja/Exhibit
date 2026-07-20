---
id: av-8xxs
status: closed
deps: []
links: [av-u0vc]
created: 2026-07-16T05:42:09Z
type: chore
priority: 3
assignee: Max Omdal
tags: [docs, terminology, renderer]
---
# Rewrite shim docs to the render-preamble taxonomy (storage adapter / capability bridge / polyfill)

The word **"shim"** is overloaded across the docs. It is used both as the *umbrella* for
everything injected into the rendered frame **and** as the name of the *storage-specific*
piece. That collision hides a real distinction: the injected pieces share only a
**delivery mechanism** (each is installed as the first `<head>` script and replaces a
browser global via `defineProperty` before any artifact code runs), not a **purpose**. By
purpose they are three distinct families:

| Family | Members | What it does | Precise name |
|---|---|---|---|
| **Storage adapter** | localStorage, sessionStorage, *(deferred: IndexedDB, `window.storage`)* | Intercept a storage API and redirect its *backing* to our server → portable, cross-device state. Surface identical, backing swapped. | *storage shim / adapter* |
| **Capability bridge** | clipboard, downloads | Re-grant a capability the sandbox *denied* by proxying the op to the trusted host under first-use approval. Not persistence. | *capability bridge* |
| **Polyfill** | File System Access (`showOpenFilePicker`/…) | Reconstruct an API *absent* in this environment atop available primitives (`<input type=file>` + the download bridge). | *polyfill* |

The docs already drift toward the right words — "storage shim," "capability bridge," and
literally "polyfill" all appear. The only real defect is that bare **"shim" doubles as the
umbrella and the storage-family name.** This chore fixes that one collision in the prose.

## Scope: documentation prose only

This is a **terminology/notes rewrite**, not a code refactor. Renaming Go identifiers
(`shimTemplate`, `injectShim`, `shimStorage`, `clipboardShim` in `internal/render/render.go`,
test names, etc.) is **explicitly out of scope** — a separate, riskier change tracked on
its own if ever wanted. Do not touch code behavior.

## The two decisions to ratify, then apply

1. **Umbrella term** for the injected bundle — *the security-sensitive JS injected as the
   first `<head>` script that replaces browser globals before artifact code runs.*
   **Proposed: "the render preamble"** (alt: "injected preamble"). Once chosen, use it
   wherever docs currently say "the shim" to mean the whole thing.
2. **Family names** — keep the three above. "Storage shim" may stay as the established
   name for the storage family (a shim classically swaps a backing behind an unchanged
   surface, which is exactly what it does) **but must no longer stand in for the umbrella.**

## Files to rewrite (prose)

Approx. current "shim" hits per file (grep, 2026-07-15) — the rewrite reconciles each to
the taxonomy above:

- `docs/product_requirement_doc.md` (~11) — §5 "the storage shim": scope to the storage family; where §5.2 lists localStorage/IndexedDB/`window.storage`, name it a **storage adapter** and note IndexedDB's async design differs (see below).
- `docs/architecture.md` (~14) — §3.2 render surface, §6 render+state flow: separate "render preamble" (the injected bundle) from "storage shim" / "capability bridge" / "polyfill" roles.
- `docs/security.md` (~9) — §1, §4: §4 already uses "capability bridge" and "polyfill"; add a short **"render preamble taxonomy"** note defining the umbrella + three families so the vocabulary has one home.
- `docs/technical_stack.md` (~5) — §6 "The storage shim": align to storage-adapter framing.
- `docs/api.md` (~2), `docs/agent.md` (~1) — incidental mentions; align wording.

## Call out IndexedDB as a distinct adapter, not "the localStorage shim again"

While rewriting PRD §5, make explicit that IndexedDB (deferred) is a **storage adapter by
goal but a different mechanism**: the localStorage shim's defining trick is *inline all
state synchronously at render* (because `localStorage` is synchronous and read at startup
before any `await`, so an async fetch would lose the race). IndexedDB is already async and
can be large, so that inlining neither applies nor scales — it needs its own adapter
design (likely lazy/bridged, not inlined). Naming it a "storage adapter" instead of "the
IndexedDB shim" surfaces that it shares the goal with localStorage but not the mechanism.

## Note the cross-cutting relationship to av-u0vc

The render-preamble umbrella cuts **across** the capability taxonomy in **av-u0vc** (the
capability-bridge registry). That registry serves the **capability-bridge family only**;
storage adapters and polyfills are separate axes it does not touch. The docs should make
that orthogonality legible so the two efforts aren't conflated.

## Acceptance Criteria

- An umbrella term is chosen and used consistently for the injected bundle; no doc uses bare "shim" to mean "everything injected into the frame."
- The three families (storage adapter, capability bridge, polyfill) are named consistently across `product_requirement_doc.md`, `architecture.md`, `security.md`, and `technical_stack.md`.
- One doc (proposed: `security.md` §4) carries a short canonical "render preamble taxonomy" definition the others can point at.
- PRD §5 states that IndexedDB is a storage adapter whose async/large-store nature makes it a different design from the inlined localStorage shim (not a copy).
- The docs note that the av-u0vc registry covers the capability-bridge family only, orthogonal to storage adapters and polyfills.
- No code identifiers, behavior, or tests are changed (identifier renames are a separate future ticket).

