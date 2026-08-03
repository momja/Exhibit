---
id: av-p4hm
status: closed
deps: []
links: [av-9jll, av-ms3r, av-st7c, av-1rvm, av-y98v, av-hh1o, av-hg5f]
created: 2026-08-01T19:00:28Z
type: epic
priority: 2
assignee: Max Omdal
tags: [state, render, ui]
---
# Web Storage: correct semantics and user management of artifact state

Artifact state — everything the storage shim captures from the Web Storage APIs — is currently a single flat, permanent, cross-device key/value map with no user-facing way to inspect or correct it, and with semantics that diverge from the Web Storage spec in ways artifacts cannot detect. This epic covers both halves of that: making the shim's behavior match what artifacts are written against, and giving the user direct management of the resulting data.

Scope boundary: this epic is about the SYNCHRONOUS Web Storage surfaces the shim already intercepts (localStorage, sessionStorage) and the artifact_state rows behind them. IndexedDB and the window.storage async API remain deferred (PRD §5.2) and are NOT in scope — note that in the framed case they do not silently fall back to per-device storage either, they throw, because the opaque-origin sandbox has no storage bucket at all (docs/security.md §—, av-yvtb stance).

Background: the shim (internal/render/render.go:233-259) installs ONE Storage object over ONE cache under both the localStorage and the sessionStorage property. That was an implementation shortcut in the original v1 drop (34c0e99), not a design decision — no doc states a rationale, and the specs only ever asked for both surfaces to be intercepted (technical_stack.md:157 says 'objects', plural). Several defects follow from it, and they compound: because the shim's own delete and clear paths are broken, the user has no working escape hatch inside the artifact, which is what makes the management UI (av-hg5f) load-bearing rather than a nicety.

## Design

Three defect fixes and two user-facing capabilities, deliberately kept as separate tickets because the fixes are behavior changes to a security-sensitive file and the capabilities are additive UI/API work.

**Correctness (bugs, linked):** separate the sessionStorage namespace from localStorage; make clear() write through instead of silently reverting on reload; make removeItem delete the row rather than leaving an empty-string tombstone. All three are invisible to the artifact author, which is why they need to be written down rather than discovered.

**Management (stories, children):** a typed state inspector on the edit page (av-hg5f), and agent-sidecar read/write access to the same state through the same API routes.

**Ordering note.** The removeItem and clear() fixes both want row deletion, which does not exist yet: the state API today is only GET and per-key PUT. av-hg5f introduces DELETE /api/artifacts/:id/state/:key and DELETE /api/artifacts/:id/state plus the Store methods behind them, so it is the natural first piece even though it is the least urgent — the bug fixes can consume its routes rather than each inventing their own.

**Migration reality.** Nothing recorded which namespace a given artifact_state row came from, so existing rows cannot be sorted into local vs session on the sessionStorage split. They all stay as localStorage rows. This is a one-way door that widens with time: every day the alias runs, more genuinely-ephemeral data is fossilized into durable cross-device state.

## Acceptance Criteria

1. sessionStorage and localStorage no longer share a namespace, and sessionStorage data is not persisted server-side.
2. clear() and removeItem have effects that survive a reload — no state resurrects itself, no key comes back as an empty string.
3. The user can inspect, correct, and erase artifact state from the edit page without hand-editing raw text.
4. The agent sidecar can read and edit the same state through the same authenticated API routes, with no second write path.
5. Documentation (PRD §5, technical_stack §6, security.md, and the agent system prompt in internal/agent/agent.go) describes the shipped semantics rather than the current pair-phrasing that permitted the alias.


## Notes

**2026-08-03T04:46:48Z**

Delivered across five PRs on feature/av-p4hm/epic.

SHIPPED
- av-9jll (#83) sessionStorage split into its own in-memory namespace via a makeStorage(initial, persist) factory; the install moved inside the framed guard so a top-level render keeps native sessionStorage.
- av-hg5f (#84) typed state inspector on the edit page, plus the DELETE state routes and Store methods the bug fixes consume.
- av-st7c + av-ms3r (#85) clear() and removeItem now delete server-side, via an explicit op ('set'|'delete'|'clear') on the host bridge rather than an empty-string sentinel.
- av-lvi1 (#86) get_state/set_state/delete_state on the agent sidecar, through the same authenticated routes; preview refresh reuses the existing htmx swap.
- av-hh1o (#87) P0 found by review AFTER the four above merged: the per-key delete URL put the key in the path, so a key of '..' normalized to the artifact delete route and let an artifact destroy itself through the host frame's token. Key now travels as a query value. Introduced by av-hg5f and fixed before ever reaching main — no released version was affected.

All 5 epic acceptance criteria met.

DEFERRED, not blocking — worth filing if they are to be acted on:
1. clear() then setItem() race: both un-awaited and unordered over the bridge, so 'reset, then write defaults' can lose the defaults. Set/set reordering was already possible; this widens it to a plausible real pattern.
2. state.js: a key queued for deletion cannot be re-added in the same session; values that do not survive JSON.parse/stringify (1e999, integers past 2^53) re-save mangled if anything else in that row is edited.
3. PRD §5.3 and the architecture.md §6 render/state diagram still show only setItem -> postMessage -> PUT, without the delete/clear directions the op protocol added.

RELATED: av-1rvm (draft) carries the state undo/snapshot/export question this epic sharpened by making state destruction actually work. av-y98v closed as superseded.
