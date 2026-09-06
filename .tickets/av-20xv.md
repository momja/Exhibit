---
id: av-20xv
status: open
deps: []
links: [av-0k5q, av-v991, av-7k7b, av-6xjd, av-8ipt]
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

**2026-09-05T16:53:24Z**

DECISION (2026-09-05): (c), and the flag gets a real meaning as a result

The design note defers option (c), a per-recipient share, to av-0k5q as a
product direction rather than a bug fix. That direction is now chosen: shares
are directed grants, the artifact keeps its single owner, and a recipient may
run it and write its state but never copy it or edit its source.

That changes what this ticket is about. `public` is unenforced today because
`public: false` has no alternative to point at: with no recipient on the row,
every share is a capability URL and there is nothing at the door to check. Once
a recipient exists the flag finally distinguishes two real things.

    public: true    anonymous link. Anyone holding the URL opens it.
    public: false   directed. Requires a named recipient, checked at the door.

So do not remove it and do not rename it to `listed`. Build it as part of the
recipient work, where it is one column beside another rather than a feature of
its own. Option (b), the listed/unlisted reading, is a different question and
should get its own field if public mode ever wants one.

**The interim fix still stands and should not wait for any of that.** Right now
the server stores an access-control flag it does not honour, which is worse than
having no flag. Reject `public: false` with a 400 naming the reason until the
recipient check exists.

**2026-09-06T01:21:09Z**

SUPERSEDED (2026-09-05, same day): drop `public`, do not wire it up

The note above concludes that a recipient column gives `public` a real meaning
(true = anonymous link, false = directed) and says to build it alongside the
recipient work. Working through the schema on av-7k7b shows that is wrong, in
the direction of removal rather than more work.

Once shares carries recipient_id, `public` is exactly `recipient_id IS NULL`.
Two columns encoding one fact, with nothing preventing them from disagreeing —
which is a worse version of the defect this ticket already describes.

So: drop it in the same migration that adds recipient_id. Same reasoning av-8ipt
applied to expires_at, and doing it while every row is still uninterpreted keeps
it a schema change rather than a data migration.

The interim fix stands until then: stop accepting a value the server does not
honour.
