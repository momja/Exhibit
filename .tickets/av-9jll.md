---
id: av-9jll
status: closed
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

**Framed (opaque origin) — give sessionStorage its OWN, purely in-memory Storage object:** separate cache, no writeThrough, no server involvement.

  Object.defineProperty(window, 'localStorage',   { value: shimStorage,     ... });
  Object.defineProperty(window, 'sessionStorage', { value: memoryStorage(), ... });

Why the real sessionStorage cannot simply be left alone here: the opaque-origin sandbox has no storage bucket, so the native sessionStorage getter throws. Something must be installed; the original shortcut was to install the object already built for localStorage.

This is not an approximation — it is EXACTLY the native semantics for this frame. A sandboxed browsing context gets a FRESH opaque origin on every navigation, and storage is keyed by origin, so native sessionStorage would also start empty after an iframe reload. There is no reload-survival gap to accept in the framed case, and no reason to bridge to the host frame's real sessionStorage (which would reintroduce the synchronous-startup-read race that render-time inlining exists to solve for localStorage). Per-frame isolation is likewise correct rather than a compromise: each sandboxed frame already has its own unique opaque origin, so two frames sharing a sessionStorage would be the wrong behavior.

**Top-level (real origin) — do NOT shim sessionStorage at all.** The install is currently unconditional (internal/render/render.go:256-259, above the `window.parent !== window` guard the bridges use), so it also replaces storage on the direct/share render at RENDER_ORIGIN/a/:id. That document has a stable real origin where native sessionStorage genuinely works, is correctly tab-scoped, and DOES survive a reload — replacing it with an in-memory object is a strict downgrade there, and it is the only place a reload-survival gap would exist. Move the sessionStorage install inside the framed guard.

Out of scope, tracked elsewhere: localStorage has its own top-level problem — the shim installs, reads inline from server state, but writeThrough returns early with no host frame, so writes are silently dropped. That is av-blzu (make direct open an explicit read-only snapshot view), not this ticket.

**Migration.** Provenance was never recorded, so existing artifact_state rows cannot be sorted into local vs session. They all remain localStorage rows. No data migration; just a note.

**Also in scope:** internal/agent/agent.go:106 currently tells the model 'localStorage and sessionStorage work and persist across the user's devices'. That is the wrong contract and must change in the same commit, or the sidecar keeps generating artifacts built against the old behavior. Same for the pair-phrasing in PRD §5.2, technical_stack.md §6, and docs/security.md.

## Acceptance Criteria

1. localStorage and sessionStorage are distinct objects over distinct caches; a key written to one is not readable from the other.
2. sessionStorage writes produce no artifact_state rows and no postMessage write-through.
3. sessionStorage data does not appear on a second device or in the state inspector.
4. A render test asserts the two namespaces are independent, covering the collision case directly.
5. The sessionStorage shim installs ONLY when framed; a top-level render at RENDER_ORIGIN/a/:id leaves native sessionStorage in place, verified by a render test asserting the shim script guards the install.
6. The agent system prompt (internal/agent/agent.go:106) and the PRD/technical_stack/security docs describe the corrected semantics: sessionStorage is frame-local and never persisted, and it is not a lossy approximation, because a sandboxed frame's opaque origin is fresh on every navigation.
7. Existing artifact_state rows are untouched — no migration attempted, and the reason is documented.


## Notes

**2026-08-02T17:22:39Z**

Verified empirically (2026-08-02), Chrome, sandbox="allow-scripts" iframe served over http — same setup as the render surface (detail.tmpl:72 / agent.tmpl:115). Probe result:

  self.origin = null
  localStorage:   THREW SecurityError — Failed to read the 'localStorage' property from 'Window': The document is sandboxed and lacks the 'allow-same-origin' flag.
  sessionStorage: THREW SecurityError — Failed to read the 'sessionStorage' property from 'Window': The document is sandboxed and lacks the 'allow-same-origin' flag.
  indexedDB:      THREW SecurityError — Failed to execute 'open' on 'IDBFactory': access to the Indexed Database API is denied in this context.

Settles the 'why shim sessionStorage at all if the frame already isolates it' question: an opaque origin does not get a PRIVATE storage area, it gets NO STORAGE KEY (storage is keyed by origin, and an opaque origin cannot produce one). So the getter throws on PROPERTY ACCESS, before any method call — leaving native sessionStorage in place means an artifact doing sessionStorage.getItem(...) at startup dies with an uncaught SecurityError and, since that is typically top-of-script, never runs at all. Hard failure, not graceful degradation. Something must be installed; this ticket only decides what.

Also confirms the epic's scope note (av-p4hm): IndexedDB does not quietly fall back to per-device storage in the framed case — it throws too.

Incidentally explains the original shortcut (34c0e99): both getters throw, so the author needed two replacements and reused the one object already built for localStorage.
