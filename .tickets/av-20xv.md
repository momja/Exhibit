---
id: av-20xv
status: open
deps: []
links: [av-0k5q, av-v991, av-7k7b]
created: 2026-08-09T04:24:30Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, sharing, api]
---
# shares.public is a dead flag: accepted, stored, never enforced

`POST /api/shares` accepts `public`, `CreateShare` writes it, `GetShare` scans it back, and the create handler logs it. **`ServeShare` never reads it.** It checks only that the row resolves and that `expires_at` has not passed (internal/render/render.go).

So a share created with `public: false` renders exactly like one created with `public: true`: anybody holding the URL gets the artifact.

This is worse than the field being absent. The API advertises an access control that does not exist, and `public: false` is precisely what a caller would set believing they had made a restricted share. The column has been there since 001 and is named in the PRD's schema sketch (§4.4) and architecture §7, so it reads as intentional rather than vestigial.

Related: there is no per-user allowlist anywhere in the schema — an artifact is owner-only, shared-by-link, or public-mode-visible, and nothing narrower. `public` looks like the beginning of that narrower thing and is not.


## Notes

**2026-08-09T04:24:30Z**

DESIGN — decide which of three, none of which is 'wire it up as-is'

**(a) Remove it.** A share is a capability URL; the link *is* the authorization (architecture.md §7). Under that model 'a non-public share' is meaningless — there is no identity at the door to check. Dropping the field makes the API honest about what it does. Cheapest, and the most consistent with the shipped design.

**(b) Redefine it as 'unlisted vs listed'.** Not an access control at all: both render to anyone with the link; `public` only decides whether the artifact appears in a public-mode gallery listing. That is a real distinction a user would want (publish my library, but this one only by direct link), it needs no identity, and it is nearly free — public mode's `ListArtifacts` filters it out. If (b) is chosen, **rename it**: `listed` says what it does, `public` will keep being read as access control.

**(c) Make it real: a per-recipient share.** Requires the ACL this codebase does not have — a recipient on the share row or a grant table, plus identities on both ends. av-30rj's `users` table makes it possible for the first time. This is the 'directed shares' half of the choice av-0k5q frames, and it should be decided there rather than here: it is a product direction, not a bug fix.

**Whichever is chosen, the interim fix is the same and should not wait:** stop accepting a value the server does not honour. Either reject `public: false` with a 400 naming the reason, or ignore it and omit it from responses. Silently storing an unenforced access-control flag is the actual defect.

Note for (b) and (c): av-wmp6 shipped public mode reading two routes (`GET /api/artifacts`, `GET /api/artifacts/:id`) as `PUBLIC_OWNER_ID`. That listing is exactly where a `listed` flag would apply, so (b) is a small addition on top of what already exists.
