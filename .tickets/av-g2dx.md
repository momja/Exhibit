---
id: av-g2dx
status: open
deps: []
links: [av-q30x, av-30rj]
created: 2026-08-09T17:26:41Z
type: epic
priority: 2
assignee: Max Omdal
tags: [multi-user, frontend, account]
---
# Epic: User account settings — the surface a person manages their own account from

There is no user-level settings surface. The templates are gallery, detail, edit, new, agent, login, notfound and partials — nothing an account is managed from. Everything user-scoped that exists is either an env var (the operator's), a per-artifact control, or hidden somewhere unrelated: the BYO agent provider key is a modal on the agent page, which is user-level settings living on a feature page because there was nowhere else to put it.

That was fine while every instance had exactly one user who was also the operator. With multi-user (av-30rj) the two separate: a person signing in through an identity provider has no relationship with the deployment and no shell on the host, so anything they need to do to their own account has to exist in the product.

The immediate driver is account deletion — a user removed at the identity provider keeps their artifacts under an owner id with no interface to reassign or delete them, and recovering or erasing a departed user's library currently means SQL (documented honestly in deployment.md §3.4). But deletion is not the only thing with no home, which is why this is an epic rather than a single story.

## Candidates for the surface

- **Delete my account and library** — the driver, and the one with a real prerequisite (blob deletion).
- **The BYO agent key** — already exists, currently reachable only from the agent page.
- **Active sessions** — av-30rj made sessions server-side rows precisely so they can be revoked; nothing exposes 'sign out my other devices'.
- **Export** — 'take your data with them' is a stated architectural principle (architecture.md §1), and a person who cannot delete or export their library does not really own it. Note Exh-avau (static build export) and av-1rvm (state export) already circle this.

## Explicitly not in scope

Administration of *other* users. The identity provider is the user directory (deployment.md §3.4); this epic is about a person acting on their own account, not an operator acting on someone else's.

