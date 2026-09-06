---
id: av-6xjd
status: open
deps: [av-lrae]
links: [av-0k5q, av-20xv, av-v991, av-7k7b, av-8ipt]
created: 2026-08-09T16:34:08Z
type: feature
priority: 2
assignee: Max Omdal
tags: [sharing, frontend, gallery, design]
---
# Sharing in the UI — design and test before building

Sharing is API-only today and stays that way for the multi-user epic — a deliberate decision (2026-08-09), not an oversight. There is no share button, no list of what you have shared, and no indication on a card that an artifact is shared at all.

The mechanism is sound: `POST /api/shares` mints a row, `GET /s/:id` on the app origin redirects to the render origin, and the render surface serves it to anyone with no credentials because the share row *is* the authorization. Verified end to end against a running instance.

What is missing is the product around it, and it needs design and testing first rather than a button bolted on:

- **No affordance to create one.** A share is currently a curl command.
- **No way to enumerate what is shared.** You cannot audit what you cannot list, and the failure mode is the share made months ago that nobody has thought about since.
- **No indication on a card.** `ListArtifacts` carries no share data at all, so the gallery could not show it even if the template wanted to.
- **A share publishes the owner's state.** `ServeShare` inlines it deliberately ('a share publishes the artifact as its owner sees it'). Fine for a stateless tool, not obviously fine otherwise — av-7k7b owns that question.

Blocked on nothing technically. Deliberately not started: the interaction design and the state question should be settled before any of it is built.


## Notes

**2026-09-06T00:44:24Z**

SCOPE INPUT (2026-09-05): the recipient is an account on this instance

The interaction design this ticket is waiting on now has its boundaries. From
av-0k5q's directed-share decision and av-7k7b's v1 scope:

- A share names a recipient, and the recipient is a users row on this instance.
  Exhibit is not decentralized. Sharing with someone who has no account here is
  out of scope for v1, so there is no invite flow, no email, and no
  account-on-first-open to design.
- The recipient runs the artifact and writes its state. Nothing else. No copy
  into their gallery, no source editing, no metadata, no re-sharing. So there is
  no "shared with me" library section to design either: nothing lands in their
  gallery to file.
- The anonymous link stays read-only in v1, which removes the "anyone with this
  link can change what is stored here" warning copy from this ticket's surface.
  It comes back if the anonymous board is ever built (av-v991).
- No expiry (av-8ipt removed the column and v1 does not add it back).

What is left for this ticket, and it is still the whole product:

1. The affordance to create a share, including picking the recipient and the
   state mode ('own' or 'shared' — av-v991).
2. Enumeration. You cannot audit what you cannot list, and a share carrying live
   state makes that worse rather than better. This is the piece most likely to
   be cut and the one that should not be.
3. The card indicator. ListArtifacts carries no share data at all, so the
   gallery could not show it even if the template wanted to. av-v991's 18:48
   note has the badge design (one badge naming the strongest thing true, private
   gets no marker) and the denormalization argument.
4. The recipient's half. Someone handed a link opted into nothing, and on a
   'shared' board their input is visible to the owner. The page has to say so in
   a sentence, not a badge.

**2026-09-06T01:01:48Z**

RULE (2026-09-05): only the owner modifies CSP, allowlists and capability
approvals

The server already enforces this and has since av-ep8k. setOriginDecision reads
ownerIDFromCtx and Store.SetOriginDecision gates on ownsArtifact before the
upsert; PATCH /api/artifacts/:id is owner-scoped the same way. A non-owner gets
404, never 403. The agent surface already lives under the same rule for the same
stated reason: agentSubResources excludes the origins route because approving an
artifact's own egress is not a decision model output gets to make.

## What breaks anyway, and it is this ticket's to fix

A recipient viewing a shared artifact gets an app-origin page with a host frame,
which is where the prompts live. Three of them write per-artifact authority:

    network permission prompt (av-kmwj)     POST /api/artifacts/:id/origins
    download / clipboard / link first-use   PATCH /api/artifacts/:id
    camera / microphone gate                PATCH /api/artifacts/:id

Hand a recipient today's detail page and all three render, and all three 404 on
submit. Click Allow, nothing happens. That teaches the recipient the tool is
flaky rather than that it is not theirs to grant.

**A recipient's session renders none of those prompts.** What they get instead
is the path av-kmwj already built for the case where approving would not help
(a redirect): explain, do not ask. "This tool tried to reach api.example.com.
Its owner has not allowed that." No button.

Extend the rule past the allowlist to the capability approvals. They are the
same per-artifact authority in different columns, and the two device flags build
the Permissions-Policy header. One asymmetry worth stating so nobody reads it as
a hole: camera and microphone still hit the browser's own native permission
prompt on the recipient's machine, so an owner's approval never hands over a
stranger's camera. It stops Exhibit blocking it; the browser is the second gate.

## The consequence to design for

A shared artifact is frozen at whatever its owner approved. If the owner's
allowlist is missing an origin the tool needs, the recipient cannot fix it and
has to go ask. That is correct — the alternative is a recipient widening
somebody else's CSP — but it fails invisibly unless the explanation above is
actually shown. A silent blank tool is the failure mode here.

**2026-09-06T01:30:11Z**

DECISIONS (2026-09-05): recipients are named by username; no "shared with me"
list in v1

## Recipient handle

A recipient is specified by username, typed. Not a picker over the instance's
accounts: a picker publishes the whole user directory to every user, which
av-utap went out of its way to avoid from a different door.

**Username is only well-defined for local accounts.** users.external_id is
`local:<normalized name>` for an account this instance issued
(auth.LocalExternalID), and that column's UNIQUE constraint already makes one
account per login name a schema invariant, so the lookup is exact and free. An
OIDC account's external_id is the provider subject — opaque, not a username.
Those rows carry email, though it is NOT NULL defaulting to '' and can be blank.

v1 resolves a typed handle as: `local:<normalized>` first, then email. No schema
change, works on a local-accounts instance and an OIDC one, and the fuzziness
stays inside one resolver.

The clean alternative is a real users.username column, unique, which would also
retire the display-name fallbacks /profile and /admin/users each carry today.
That is account-management work (av-g2dx, av-utap) rather than sharing work, so
it is deliberately not pulled in here.

**This field confirms existence, on purpose.** Answering "added" for a username
nobody holds means the owner believes their friend has access and the friend
does not, which is worse than the leak. So it leaks one bit per guessed name,
where a picker would hand over the entire list for nothing. Rate limiting is the
answer if that ever matters.

**Granting is bulk.** Several usernames, one submit, per the group-chat case on
av-7k7b.

## No "shared with me" list in v1

Deferred, not rejected. A recipient keeps the artifact URL the owner sent them.

What that costs, stated so it is a known debt rather than a surprise: a
recipient who loses the link loses the artifact, which is a small version of the
"digging through chat logs" problem the PRD opens by rejecting. A list of grants
is not a copy and claims no ownership, so it stays compatible with av-0k5q's
one-owner rule whenever it is built.

**2026-09-06T01:34:21Z**

SLICE 4 OF SHARING V1 (2026-09-05): this ticket's premise is satisfied, build it

The title says 'design and test before building' and the body says it is
deliberately not started until the interaction design and the state question are
settled. Both are settled as of today. This is now the build ticket for the
owner's share panel, and it depends on av-lrae (the grant schema).

## What to build

A share panel on the artifact detail page, owner-only, with two independent
halves.

**Grants.** A username field taking SEVERAL names and one submit — 'gave friends
access, dropped the link' is one gesture (av-7k7b). Below it, the list of people
currently granted, each with a revoke. A grant is a row; revoking is a DELETE.

Resolve a typed name as `local:<normalized>` first, then email. The field
confirms existence deliberately: answering 'added' for a username nobody holds
means the owner believes their friend has access and the friend does not.

**The public link.** A toggle, not a create button: on mints the single
anonymous row and shows the URL, off deletes it and the URL dies. Plus a
'replace link' action that deletes and re-mints in one step and says the old URL
stops working, because a leaked link wants rotating and toggling off-then-on
leaves the user unsure it worked.

**The state mode** (av-v991) is one control per artifact, not per grant: 'each
person keeps their own data' or 'everyone shares one copy'. It reads on this
panel and writes artifacts.share_state_mode.

## Enumeration — the piece most likely to be cut, and the one that should not be

You cannot audit what you cannot list, and a grant carrying live state makes
that worse. ListArtifacts carries no share data at all today, so the gallery
cannot show it even if the template wanted to. av-v991's 2026-08-06 18:48 note
has the badge design (one badge naming the strongest thing true, private gets no
marker) and the denormalization argument (a count on artifacts kept current the
way tags_text is, rather than a join on every gallery render — av-b6o9 is the
precedent).

## Not in this ticket

The recipient's own view is av-awr4. A 'shared with me' list is deferred
entirely; a recipient keeps the URL the owner sent them.
