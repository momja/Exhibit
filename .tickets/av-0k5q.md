---
id: av-0k5q
status: open
deps: []
links: [av-v991, av-ec0t, av-7k7b, av-20xv]
created: 2026-08-06T21:59:24Z
type: feature
priority: 3
assignee: Max Omdal
tags: [gallery, ]
---
# design,sharing,frontend

Raised as 'maybe the gallery needs tabs — shared with me / my artifacts / tags'. Capturing the analysis, because two of those three are not the same kind of thing and one has no data behind it.

**Tags is not a peer of the other two.** 'My artifacts' and 'shared with me' are *provenance* (whose it is). 'Tags' is *organization* (how it is filed). Tabs partition a set; filters narrow one. A tags tab has to answer 'tags across which set?', and the answer being 'both' is what shows it belongs on the other axis. Tags and collections are already filters and should stay filters.

**'Shared with me' has no data behind it.** `shares(id, artifact_id, public, expires_at)` names no recipient — a share is an anonymous capability URL, not a relationship. Nothing today can populate such a view.


## Notes

**2026-08-06T21:59:24Z**

DESIGN

## Two products wear this name; pick deliberately

**(a) Directed shares.** `shares` gains a recipient user; A shares artifact X with user B; it appears in B's library. Needs identity on both ends (av-30rj supplies it), plus an invite/addressing surface. This is Drive-shaped: sharing becomes a permission grant on an object that stays A's.

**(b) Saved shares.** Shares stay anonymous capability URLs. The *recipient* saves one into their own library. No coordination, no account for the sender to address, no invite flow.

**Recommend (b).** It fits the thesis the PRD opens with — a read-it-later library, the user's home shelf, 'full control over their artifacts'. (a) drifts the product toward collaborative document permissions, which §9's non-goals do not contemplate.

## Copy or reference — the PRD already answers this

If you save someone's shared artifact, do you get a copy or a live pointer? Spec §9: *'No live-linked imports. URL-paste ingest exists (§8.1) but is a one-time vendoring fetch — after ingest the file is owned and served locally, never hot-linked or auto-synced.'*

Saving a share is the same act as pasting a URL, so it **copies**, by the rule the product already committed to. That is also what preserves the durability promise: a reference breaks the day the sender deletes theirs or the share expires, which is precisely the failure §1 exists to prevent.

## The tension with av-v991, and what it reveals

A copy forks the chess board — the opposite of what shared state is for. So a shared-state artifact is **not a library item you saved**; it is a **live session you joined**. Two different objects:

- a saved share → a file you now own, vendored, durable, yours to edit
- a shared-state artifact → something you participate in and do not own; it has no meaning detached from the other party

Folding both into one 'shared with me' list is the mistake to avoid. If sessions need a home in the UI at all, it is a different surface from the library — and possibly no surface, just the link.

## Recommendation on the UI question as asked

**Do not add tabs yet.** Add provenance as a *filter* beside tags and collections; promote it to a partition only when there is enough not-yours content to justify one. Today there is none, because neither (a) nor (b) exists. Tabs added ahead of the content they organize become chrome that never earns itself, and the gallery already carries search, tag and collection filters that a fourth dimension joins cleanly.

Sequencing: this ticket is blocked on nothing technically, but it should not be built before the (a)/(b) decision, and (b)'s save action wants the ingest path it would reuse (`POST /api/artifacts`, already the single write path) rather than a new one.
