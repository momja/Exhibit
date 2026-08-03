---
id: av-ms3r
status: in_progress
deps: [av-hg5f]
links: [av-9jll, av-st7c, av-p4hm, av-hh1o, av-hg5f]
created: 2026-08-01T19:01:28Z
type: bug
priority: 2
assignee: Max Omdal
tags: [state, render]
---
# storage shim removeItem writes an empty string instead of deleting the key

internal/render/render.go:241 —

    removeItem: function(key) { delete cache[key]; writeThrough(key, ''); }

The key is dropped from the in-memory cache but the server row is SET TO AN EMPTY STRING rather than deleted. Within the page the removal looks correct; after a reload the render re-inlines key -> '' and the key is back as a tombstone. Consequences, all of which break ordinary artifact code:

1. getItem(k) === null is false — it returns ''. This is the single most common existence check in Web Storage code.
2. JSON.parse(getItem(k)) throws on '' where the artifact expected a null it could branch on.
3. length and key(n) enumerate Object.keys(cache), so tombstones inflate the count and surface in every for (let i = 0; i < storage.length; i++) loop.
4. The state inspector (av-hg5f) will display phantom keys with empty values and cannot distinguish them from genuinely-empty values a user meant to store.

Web Storage defines removeItem as removing the key/value pair. This does not.

## Design

removeItem must delete the row. Depends on the per-key delete route av-hg5f introduces (DELETE /api/artifacts/:id/state/:key) — the state API is GET plus per-key PUT today, with no delete of any kind.

Same protocol point as the clear() bug (av-st7c): the host bridge message needs an explicit op so a delete is unambiguously distinct from setting a key to ''. Storing an empty string is legitimate and must remain possible — that is precisely why the current sentinel is wrong and why the fix cannot be 'treat empty string as delete'.

**Existing tombstones.** Rows already written as '' by the current code are indistinguishable from intentional empty values, so they cannot be cleaned up automatically. Leave them; the state inspector gives the user a way to delete them by hand, and this ticket's note is the record of why they exist.

## Acceptance Criteria

1. removeItem deletes the artifact_state row; after a reload getItem returns null, not ''.
2. Setting a key to an empty string still stores an empty string and is not treated as a delete.
3. length and key(n) do not enumerate removed keys after a reload.
4. Delete is an explicit op on the host bridge, not an empty-value sentinel.
5. A test covers removeItem-then-reload asserting getItem(k) === null, and setItem(k, '')-then-reload asserting getItem(k) === ''.
6. Pre-existing empty-string rows are left alone, with the reason recorded.

