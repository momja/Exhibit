---
id: av-8ipt
status: closed
deps: []
links: [av-20xv, av-6xjd]
created: 2026-08-09T16:34:08Z
type: chore
priority: 2
assignee: Max Omdal
tags: [sharing, schema, cleanup]
---
# Remove shares.expires_at — built ahead of need, no caller

`shares.expires_at` has existed since migration 001 and is genuinely enforced — `ServeShare` returns 410 Gone past the deadline. Unlike `shares.public` (av-20xv) it does not lie. But nothing sets it: there is no UI, and no identified use case.

It is the same category as `public` in origin — schema written ahead of the product — differing only in that this one works. That makes it non-urgent, not justified.

The decision to make: remove it, or keep it unexposed.


## Notes

**2026-08-09T16:34:08Z**

RECOMMENDATION: remove it, and do so now rather than later.

The case for removal:

- **No caller, no UI, no users — so removal is free today and never gets cheaper.** The moment anything sets an expiry, dropping it becomes a data question instead of a schema one.
- **'We might want it later' is the reasoning we are trying not to use.** If a real expiry requirement appears, re-adding is one migration — and it would then be designed against that requirement rather than retrofitted to a guess made before the product existed.
- **An unused field in a public API contract is surface nobody exercises end to end.** A caller can set it today and get behaviour no page ever produces and no user has ever seen.

The one thing removal makes worse, stated plainly: a share becomes permanent until explicitly deleted, and the forgotten-share problem gets sharper. That is honest rather than harmful — a clearly permanent share is easier to reason about than a nominally-expirable one nobody ever sets. If that problem is worth solving, it should be solved deliberately (enumerate what is shared, show it on the card) rather than by leaving a dial nothing turns.

Rejected middle option: keeping the column while removing it from the API. That is the worst of both — it retains the schema and the enforcement code while removing the only way to reach it.

Scope: drop the column (SQLite 3.35+ supports ALTER TABLE DROP COLUMN; confirm the pinned modernc.org/sqlite build does before assuming a table rebuild), drop the expiry check and its 410 path in `ServeShare`, drop the field from the create-share request and response, and update the PRD §4.4 schema sketch and architecture §7 which both name it.

Note the contrast with av-20xv: `public` is a defect because it advertises an access control it does not implement, and needs a decision regardless. `expires_at` is merely unused. Different problems, and this one should not be bundled into that ticket.

**2026-08-09T17:44:13Z**

Done. Two implementation decisions worth recording:

1. DROP COLUMN, not a table rebuild. The pinned modernc.org/sqlite v1.51.0 reports sqlite_version 3.53.1, well past the 3.35 that introduced ALTER TABLE ... DROP COLUMN, and shares.expires_at is referenced by no index, view, trigger, or constraint. Migration 015 is therefore one statement. Its Down re-adds the column empty, with a note in the file that dropped values are unrecoverable — the honest reverse, and lossless in fact because every value was NULL.

2. POST /api/shares REJECTS expires_at with 400 rather than ignoring it. Silently discarding a field the caller set is the defect av-20xv exists to fix; a caller asking for a deadline would otherwise get a permanent link and no way to know. The check is a json.RawMessage tombstone field whose presence — any value, including null — is the error, so there is one rule with no sub-cases. Nothing in the repo sends the field, so nothing breaks.

Migration 015 is covered both ways: staged at 014 with a dated share row, then migrated (column gone, both share rows intact, Down/Up round trip), and a fresh database asserted to land in the same schema.
