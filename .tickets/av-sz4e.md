---
id: av-sz4e
status: closed
deps: []
links: [av-q30x, av-30rj, av-g2dx]
created: 2026-08-09T17:39:50Z
type: epic
priority: 1
assignee: Max Omdal
tags: [multi-user, auth, backend, self-host]
---
# Epic: Built-in user backend — self-hosted multi-user without an identity server

**This reverses a decision, deliberately.** av-30rj declined local password auth as a first-class feature, reasoning that it drags in hashing, reset mail (and therefore SMTP), verification, rate limiting and eventually MFA — and concluded: delegate to an IdP, or stay single-user. av-q30x then built one credential from the environment for the single-user case.

The reversal: **self-hosted multi-user is a supported story, and shipping a user backend is the norm for self-hosted software.** Immich, Nextcloud and Vaultwarden all ship one *and* support OIDC. The BYO-identity path is the escape hatch for people who already run a provider, not the only way in. Telling someone who wants Exhibit for their household to stand up Authentik first is a bar this project should not set.

What makes this tractable now is that av-30rj's objection mostly dissolves under **operator-provisioned accounts with no email**:

| av-30rj's objection | Under this epic |
|---|---|
| Password hashing | already solved — bcrypt shipped with av-q30x |
| Reset mail / SMTP | avoided — an admin resets from the UI or CLI |
| Email verification | avoided — the admin vouches by creating the account |
| Rate limiting | **real, still missing, and in scope here** |
| MFA | still absent, as it is for today's single credential |

Only rate limiting survives, and it is a bounded piece of work rather than a product surface.

## Consequence: this epic owns administration

av-g2dx (user account settings) explicitly excludes 'administration of other users', on the grounds that the identity provider is the user directory. That holds when there *is* a provider. Once Exhibit issues credentials, Exhibit is the directory, and somebody has to create accounts and reset forgotten passwords. That belongs here.

The division: **av-g2dx is a person acting on their own account; this epic is an admin acting on the instance.**

## Shape

- Credentials live on `users`, nullable, so OIDC identities (which have none) and local ones coexist in one table and one `owner_id` space.
- The first identity to sign in becomes an admin — continuous with today's behaviour, where the first login adopts owner 1 and the existing library.
- av-q30x's `LOGIN_USERNAME`/`LOGIN_PASSWORD_HASH` is **not** discarded: it becomes bootstrap and break-glass, the way Vaultwarden's admin token does. It is how you create the first admin, and how you get back in having locked yourself out.
- No self-registration and no SMTP in v1. Both are additive later if wanted; neither is needed for the household and small-team case this targets.

## Not in scope

Groups, per-user quotas, sharing between accounts, MFA, SSO-plus-local account linking.

