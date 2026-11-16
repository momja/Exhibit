---
id: Exh-fkla
status: in_progress
deps: []
links: []
created: 2026-11-05T03:54:02Z
type: feature
priority: 0
assignee: Max Omdal
parent: Exh-ivro
tags: [bazarre, security]
---
# Keep imported remix candidates private until explicitly kept

Add persisted Bazarre draft isolation for all imports and update candidates. Drafts appear only to their owner until explicit owner POST Keep in gallery; this clears the draft flag, does not replace/delete parent or copy state. Filter drafts out of anonymous public-mode list/search and metadata/source/page reads. Anonymous or guest render/widget/share/export routes cannot expose drafts, including old/forged tokens. Owner render and agent remain available. Sharing a draft is refused until kept; duplicates retain draft status. Creator publication of a draft is refused. Normal already-kept artifact policy is unchanged. Store baseline incorporates draft flag where relevant; candidate creation never changes original. A normal initial copy can Keep in gallery immediately; remix can review/test before keeping. Migrations preserve existing artifacts as not drafts.

## Acceptance Criteria

Anonymous public gallery/API/search/detail/render/widget/share/export cannot read drafts; guest grants and prior tokens cannot bypass; owner can remix/test; explicit Keep in gallery persists visibility without deleting parent; duplicate draft remains private; tests cover public mode and all changed surfaces.


## Notes

**2026-11-16T07:14:43Z**

Coordination check after Exh-lmor: public-mode list/search/detail filtering and public-mode tests in the description/AC refer to a retired principal. Library reads remain owner/grant-scoped; unauthenticated reads require credentials. Draft isolation still applies to guest grants, share/export/widget/render paths and old/forged tokens. No dependency or scope change needed; interpret the public-mode wording as superseded.
