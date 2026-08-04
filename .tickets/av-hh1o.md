---
id: av-hh1o
status: closed
deps: []
links: [av-ms3r, av-hg5f, av-p4hm]
created: 2026-08-03T03:58:45Z
type: bug
priority: 0
assignee: Max Omdal
tags: [security, state, api]
---
# Artifact self-deletion via '..' state key — client builds a traversable DELETE path

An artifact can delete itself. The per-key state delete URL is built client-side as '/api/artifacts/{id}/state/' + encodeURIComponent(key). encodeURIComponent does NOT escape '.', so a key of '..' survives encoding, and the browser's URL parser resolves the dot segment BEFORE sending:

  ''       -> /api/artifacts/abc/state/   (404)
  '.'      -> /api/artifacts/abc/state/   (404)
  '..'     -> /api/artifacts/abc/         (DELETE ARTIFACT)

That path matches r.Delete("/", ro.deleteArtifact) (internal/api/api.go:89). So localStorage.removeItem('..') inside a sandboxed artifact causes the HOST frame — which holds the bearer token — to delete the artifact, cascading its tags, collections, shares and state. The .catch(function(){}) on the bridge fetch swallows every trace; the user just finds the artifact gone.

This crosses the innermost trust boundary in architecture.md §4: untrusted artifact code reaching a destructive mutation it was never granted. Only '..' is reachable (a literal '/' still encodes to %2F), so it is bounded to self-deletion rather than cross-artifact reach — but that is enough to lose the user's work with no confirmation and no recovery (there is no state or artifact undo; av-1rvm).

Same defect in three places, because the URL construction is duplicated:
  web/gallery/detail.js:57   (storage bridge listener)
  web/gallery/agent.js:479   (storage bridge listener)
  web/gallery/state.js:492   (state inspector per-key delete)

The state inspector path is reachable by the user too: deleting a key literally named '..' from the edit page deletes the artifact and reports success.

SERVER IS NOT AT FAULT. pathParam decodes %2E%2E back to '..' correctly; the normalization happens in the browser before the request exists. Verified against chi v5.3.0 with this route tree.

Second, related defect (same root cause, same three sites): keys '' and '.' normalize to a trailing slash, 404, and never delete. The catch hides the failure, the in-memory cache looks empty, and the next render re-inlines the row — the exact resurrection av-ms3r AC 1 forbids. Reproduce with: localStorage.setItem('', 'x'); localStorage.removeItem('').

## Design

Prefer the root-cause fix over escaping. Patching encodeURIComponent(key).replace(/\./g,'%2E') at all three sites closes the traversal but leaves the empty-string key broken (there is no such thing as an empty path segment) and leaves the key length-bound by the request line — a key large enough to SET can 414/431 on DELETE behind a proxy, since set carries the key in a JSON body and delete carries it in the URL.

Taking the key out of the path fixes all three at once. Suggested shape:

  DELETE /api/artifacts/:id/state?key=<encoded>   one key
  DELETE /api/artifacts/:id/state                 all state

r.URL.Query().Has("key") distinguishes present-but-empty (delete the empty-string key — legitimate) from absent (erase all), so the empty key stops being unrepresentable. No path segment means no dot-segment normalization to defend against.

This is cheap NOW and expensive later: the routes were introduced by av-hg5f on the unmerged epic branch and have never been on main, so there are no external consumers. Callers to update: web/gallery/{detail,agent,state}.js and internal/agent/ext/exhibit.ts.

While there, consider the duplication that made this a three-site fix (a shared stateURL/bridge helper), and note both bridge listeners treat any unrecognized op as a set rather than ignoring it, so a future op typo silently becomes a write.

## Acceptance Criteria

1. An artifact calling localStorage.removeItem('..') does not delete the artifact, and does not reach any route other than the state delete.
2. Keys '', '.', and '..' round-trip: set, read back, remove, and after a reload getItem returns null for each.
3. A key containing '/' or non-ASCII still deletes correctly (no regression on the existing percent-encoding tests).
4. Erase-all remains distinguishable from deleting the empty-string key.
5. A test covers the '..' case explicitly at the client URL-construction layer, not only server-side — the server was never wrong.
6. All three client sites are fixed, and the fix is not independently reimplemented in each.

