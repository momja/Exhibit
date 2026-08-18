---
id: av-2p8z
status: open
deps: []
links: [av-10bw]
created: 2026-08-18T05:57:40Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-1in5
tags: [hosted, backend, api, account]
---
# Per-owner entitlements: plan and limits as data an admin can set

Quotas ([[av-10bw]]) need a limit per owner, and nothing carries one. Every user is entitled to exactly the same thing today, which is everything.

This ticket adds that data and the control surface for it, and deliberately stops there. It knows nothing about payments, subscriptions, or why an owner is on the plan they are on — it stores the answer and lets an admin set it.

That is not a stub standing in for something commercial. On a self-hosted instance it is the feature outright: a household or small-team instance can give one person a larger allowance than another, which is unaskable today. Anything that maintains these values from an external system is an ordinary API client, outside this repo.

## Design

**Entitlements live on `users`.** A plan label, the limits themselves, and an opaque external reference. The label is for display and for grouping; the limits are what gates actually read, stored per user rather than derived from the label, so an instance can grant one person more without inventing a plan for them.

**Set through the admin route that already exists.** `updateUserRequest` (`internal/api/admin.go:251`) is already nullable-pointer-per-field — `Password`, `Disabled`, `IsAdmin` — and new fields extend it in the same shape. No new route group, and no new credential: `adminOnly` already grants the static service token full authority, which is what an out-of-tree client would authenticate with. The single write path is preserved for exactly the reason `architecture.md` §3.6 gives for the future Chrome extension — an external system that maintains these values is just another API client.

**Emphatically not on `/profile`.** This is the boundary §3.8a draws, and it is the one that matters here: `/profile` reaches your own account and a session is the whole authorization, while `/admin/users` reaches other accounts and requires an admin. An entitlement a person can set on themselves is not a limit. It belongs on the admin side by the same rule that puts password resets there, and a test should say so rather than leaving it to whoever adds the next profile field.

**One resolution function, and gates call it.** `av-10bw` and anything after it ask "what is this owner allowed" and never read the columns directly.

**Fail closed on ambiguity, never on absence — the distinction is the whole design.** Two states that look alike and are not: *limits are not in use on this instance*, and *limits are in use but this owner's could not be resolved*. The first must stay unlimited, because a self-hoster who upgrades must never meet a ceiling they did not ask for. The second must refuse.

So enforcement is a single explicit switch. Off — the default, and every self-hosted instance — resolves to unlimited and nothing is ever refused. On, and the instance **fails at startup** if no default entitlement is configured, rather than booting into a state where every unprovisioned signup is unlimited. That is the posture `LOGIN_USERNAME` without `LOGIN_PASSWORD_HASH` already takes (`deployment.md` §3.2): a half-configured gate fails where the operator is watching instead of quietly not being a gate. At request time, an entitlement that cannot be resolved — a database error, a row that makes no sense — refuses the write rather than waving it through, because with limits switched on, "I don't know what you're allowed" is not a reason to allow anything.

A startup *warning* was the weaker version of this and is deliberately rejected: warnings scroll past, and the failure mode they guard is one nobody notices until it is expensive.

**Non-default entitlements must be visible.** An entitlement maintained by an external system can drift from that system's view of reality — a downgrade it failed to deliver leaves someone on a raised ceiling indefinitely. Keeping them current is that system's job, not this repo's, but *seeing* them is not: a way to list owners whose entitlement differs from the default is what makes drift discoverable at all, and it belongs beside the admin screen that sets them.

**The external reference is opaque and carries no vendor semantics.** It is a string an operator's own system uses to recognize an account, in the spirit of a ticket's `--external-ref`. It lives here rather than in the external system because it is durable with the account: the account survives that system being rebuilt, replaced, or dropped.

**What is deliberately out of tree.** Anything that decides *why* an owner has a given entitlement — payment state, subscriptions, invoices, a provider's webhook and its signature scheme. None of it is in this repo, in any form, including as an interface with a stub implementation. Two reasons beyond the obvious one. Go's `internal/` rule blocks an external module from importing `internal/api`, so the conventional seam-plus-private-implementation shape would force promoting `api.Config` to a public package — a permanent API-surface commitment made for a packaging reason. And an empty `billing` package in a public tree discloses roughly what naming a vendor would, while being useless to everyone who reads it.

## Acceptance Criteria

- `users` carries a plan label, per-owner limits, and an opaque external reference; account deletion removes them with the row.
- An admin sets all of them through the existing admin user route; a non-admin gets the same `404` `adminOnly` gives every other administrative address.
- `/profile` cannot set or raise any of them, and a test enforces it.
- One resolution function answers "what is this owner allowed"; no gate reads the columns directly.
- With enforcement off — the default — every owner resolves to unlimited and no request is refused; a self-hosted instance that configures nothing behaves identically to today.
- With enforcement on and no default entitlement configured, the server fails at startup rather than booting unlimited.
- With enforcement on, an entitlement that cannot be resolved refuses the write rather than allowing it, and the failure is logged.
- Owners whose entitlement differs from the default can be listed.
- No payment provider, subscription concept, or vendor name appears anywhere in the repo, including in tests, fixtures, docs, and `go.mod`.
- `docs/deployment.md` documents the fields as operator-set, and records that maintaining them from an external system is an ordinary authenticated API client.

## Notes

**2026-08-18T06:12:26Z**

Rewritten to remove the payment vendor entirely. The ticket now covers per-owner entitlements as data plus the admin control surface, and stops there; whatever decides why an owner is on a plan is an ordinary API client living outside this repo. Not an interface with a stub either — Go's internal/ rule would force promoting api.Config to a public package for packaging reasons, and an empty billing package discloses about as much as naming a vendor.

**2026-08-18T06:25:14Z**

Fail-closed on ambiguity, never on absence. Enforcement is one explicit switch: off (the default, every self-hosted instance) is unlimited; on with no default entitlement configured fails at startup rather than booting unlimited; an unresolvable entitlement at request time refuses. A startup warning was rejected as the weaker version — warnings scroll past.
