---
id: av-9jll
status: open
deps: []
links: [av-ms3r, av-st7c, av-p4hm]
created: 2026-08-01T19:00:53Z
type: bug
priority: 2
assignee: Max Omdal
tags: [state, render, security]
---
# sessionStorage is aliased to localStorage — shared namespace and wrong lifetime

internal/render/render.go:233-259 builds one shimStorage object over one cache and installs it under BOTH window.localStorage and window.sessionStorage. The two Web Storage areas are distinct namespaces with different lifetimes; artifacts are written against that. Merging them produces four defects, none of which an artifact author can detect:

1. **Key collisions.** sessionStorage.setItem('draft', …) and localStorage.setItem('draft', …) are conventionally different data (this session's scratch vs. the saved copy). Here the later write clobbers the earlier one and a read from either name returns whichever won.
2. **Lifetime inversion.** sessionStorage means 'dies with the tab'. Under the shim it is permanent AND synced cross-device. Artifacts choose sessionStorage precisely for things that must not survive — a dismissed-banner flag, a wizard's in-progress step, a one-shot intro. Dismiss a banner on the phone and it is gone on the Mac too, permanently, with no in-artifact undo (and no working clear(), see the sibling bug).
3. **Tab isolation lost.** sessionStorage is per-tab by definition; two tabs on one artifact now share a single server-backed map under last-write-wins and overwrite each other.
4. **State pollution.** Ephemeral data accumulates as durable artifact_state rows, which the user then has to manage in the state inspector.

Not a design decision: the alias arrived with the original v1 commit (34c0e99) and no doc gives a rationale. The specs only ever asked that both surfaces be intercepted — technical_stack.md:157 says 'objects' (plural) implementing the Storage interface.

## Design

Give sessionStorage its OWN, purely in-memory Storage object — separate cache, no writeThrough, no server involvement:

  Object.defineProperty(window, 'localStorage',   { value: shimStorage,     ... });
  Object.defineProperty(window, 'sessionStorage', { value: memoryStorage(), ... });

This is not a compromise; it is closer to the spec than today's behavior. sessionStorage is scoped to the tab and discarded with it, and an in-memory object in a frame that dies with the page is very nearly that, at zero cost — no rows, no write-through, nothing in the state inspector.

**Why the real sessionStorage cannot simply be left alone:** the opaque-origin sandbox has no storage bucket, so the native sessionStorage getter throws. Something must be installed; the original shortcut was to install the object already built for localStorage.

**Known fidelity gap (accept and document).** Real sessionStorage survives a RELOAD of the same tab; an in-memory object does not. Closing that would mean bridging to the host frame's real sessionStorage over postMessage, which reintroduces the synchronous-startup-read race that render-time inlining exists to solve for localStorage — and the host cannot inline into a server-rendered document. So: in-memory, gap accepted, written down in docs/security.md.

**Migration.** Provenance was never recorded, so existing artifact_state rows cannot be sorted into local vs session. They all remain localStorage rows. No data migration; just a note.

**Also in scope:** internal/agent/agent.go:106 currently tells the model 'localStorage and sessionStorage work and persist across the user's devices'. That is the wrong contract and must change in the same commit, or the sidecar keeps generating artifacts built against the old behavior. Same for the pair-phrasing in PRD §5.2, technical_stack.md §6, and docs/security.md.

## Acceptance Criteria

1. localStorage and sessionStorage are distinct objects over distinct caches; a key written to one is not readable from the other.
2. sessionStorage writes produce no artifact_state rows and no postMessage write-through.
3. sessionStorage data does not appear on a second device or in the state inspector.
4. A render test asserts the two namespaces are independent, covering the collision case directly.
5. The agent system prompt (internal/agent/agent.go:106) and the PRD/technical_stack/security docs describe the corrected semantics, including the reload-survival gap.
6. Existing artifact_state rows are untouched — no migration attempted, and the reason is documented.

