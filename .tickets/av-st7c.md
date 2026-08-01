---
id: av-st7c
status: open
deps: [av-hg5f]
links: [av-9jll, av-ms3r, av-p4hm]
created: 2026-08-01T19:01:12Z
type: bug
priority: 2
assignee: Max Omdal
tags: [state, render]
---
# storage shim clear() is cache-only — wipe silently reverts on reload

internal/render/render.go:245 —

    clear: function() { cache = {}; }

No writeThrough. The in-memory cache is emptied so the wipe looks successful for the rest of the page's life, but nothing is deleted server-side; the next render re-inlines every original key and all the state comes back. An artifact offering a 'reset everything' / 'clear my data' button is therefore broken in a way neither its author nor its user can detect until a reload — and 'it worked, then it didn't' is worse than an outright failure, because the user believes their data is gone.

Compounded by the sibling aliasing bug (av-9jll): calling clear() on what the artifact thinks is sessionStorage also wipes the localStorage cache, since they are the same object.

Practical consequence for the epic: there is currently NO working reset path inside an artifact, which is what makes the state inspector's 'Erase all data' button (av-hg5f) the only functioning escape hatch in the product rather than a convenience.

## Design

clear() must delete every key for the artifact server-side, not just drop the cache.

Depends on the bulk-delete route av-hg5f introduces (DELETE /api/artifacts/:id/state) — the state API today is only GET and per-key PUT, so there is nothing for clear() to call. Either post a single clear message over the existing host bridge and let the host issue that DELETE, or iterate keys and reuse the per-key path; the single message is preferable, as key-by-key is O(n) postMessages and races a concurrent write.

The bridge protocol needs a shape for this: today's message is { __avState, artifactId, key, value } and a clear has no key. Add an explicit op field rather than overloading key/value with a sentinel — a sentinel here is the same class of mistake as removeItem's empty-string tombstone (av-ms3r), and the host must be able to tell 'clear everything' from 'set a key to something falsy' with no ambiguity.

Note the same top-level guard the other bridges use: with no host frame there is nothing to persist through, so clear() stays cache-only there, consistent with the rest of the shim.

## Acceptance Criteria

1. clear() removes every artifact_state row for the artifact; a reload shows no state.
2. clear() is distinguishable from a key write at the host bridge — an explicit op, not a sentinel key or value.
3. Calling clear() top-level (no host frame) does not error and remains cache-only, matching the other bridges' guard.
4. A test covers clear-then-reload, asserting state does not resurrect.

