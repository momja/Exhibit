---
id: Exh-u9b1
status: in_progress
deps: [Exh-wcxf]
links: []
created: 2026-10-26T01:02:13Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-qeuv
tags: [gallery, sharing, frontend]
---
# Gallery pages for Shared with me and Shared by me

Serve the two new spaces through the existing gallery machinery (internal/api/gallerygrid.go).

- galleryDefinition currently carries static store.ListOptions and the owner always comes from the request. Generalize it so a definition supplies its own *selection* (e.g. a load func or an interface: list(ctx, r, offset, limit, query) -> cards) while paging, lookahead, search, HX-Replace-Url and the error/retry fragment stay shared. Keep libraryGallery's behavior byte-identical.
- Routes (page group under ownerMiddleware, pageowner_test must still pass): GET /shared + /partials/gallery-grid/shared (Shared with me), GET /shared-by-me + /partials/gallery-grid/shared-by-me. The '/' library stays as is.
- Shared-with-me cards: show the owner (small avatar initial + name line under the title, per the mockup's '.by' row), open the ordinary /artifacts/:id read-only detail page (av-awr4), widget tiles render through the recipient's viewer token exactly as the detail page does (check /artifacts/:id/widget/open accepts a recipient; if it is owner-only, add the readable-by path rather than widening it). The action menu offers only actions a grant carries: no Edit, no Duplicate, no Delete. Capability/share glyph cluster: recipients should not see the owner's sharing glyph (it describes the owner's audit, not theirs) — show none.
- Shared-by-me cards: identical to library cards (they are the owner's own artifacts).
- Empty states: Shared with me — 'Nothing has been shared with you yet.' with no action; Shared by me — explains sharing happens from an artifact's share panel.
- Owner filter for Shared with me via ?owner=<id> (repeatable), validated against SharedWithOwners; paging/search URLs preserve it.
- Template + Go tests per existing gallerygrid_test.go patterns; one Chromium browser test that a recipient sees a granted artifact, opens it, and loses it after revocation.

## Acceptance Criteria

- /shared and /shared-by-me render, page, and search with the shared gallery partials.
- Recipient cards have owner attribution and no owner-only actions.
- Library '/' output unchanged (existing tests untouched).
- go test ./... and the Node suite pass; screenshots updated with shot-scraper for any changed page.

