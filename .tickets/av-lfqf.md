---
id: av-lfqf
status: open
deps: []
links: []
created: 2026-08-11T06:21:13Z
type: chore
priority: 2
assignee: Max Omdal
tags: [architecture, store, tech-debt]
---
# Split the Store god interface by subsystem

internal/store/store.go's Store interface has grown to 49 methods spanning ~11 subsystems (artifacts, network origins, collections, tags, state, agent transcripts, shares, users, sessions, credentials, admin) -- the file's own '--- section ---' comment headers are the split trying to happen. Every method is documented twice (once on the interface, once on the SQLite implementation).

The stated justification for the interface is a swap seam (SQLite -> something else later, per technical_stack.md's evolution table), but there is no fake Store anywhere in the codebase: all ~40 test files that touch the store spin up a real store.OpenSQLite() on a temp file. So the interface currently pays the full tax (double documentation, one chokepoint every new feature must extend) without buying the thing an interface usually buys -- a lightweight test double.

The escape hatches are a tell that the interface has grown past its original scope: GetArtifactUnscoped and GetShareUnscoped exist because the interface accreted HTTP/render-layer concerns (owner scoping bypass for the render surface and the share path) that arguably belong one layer up, not on the store.

## Acceptance Criteria

- Either: split Store into per-subsystem interfaces (e.g. UserStore, ArtifactStore, StateStore, AdminStore, ...) so callers like internal/render and internal/agent depend only on the slice they use, with the SQLite type composing/implementing all of them -- OR: write and land a working fake/in-memory Store implementation that justifies keeping one interface, with at least one real caller (e.g. a currently-slow store test suite) converted to use it to prove it's exercised.
- No behavior change; this is a structural refactor only.
- GetArtifactUnscoped / GetShareUnscoped's placement is reconsidered as part of the split (either they land on a render-specific narrow interface, or a documented reason is recorded for why they stay general).

