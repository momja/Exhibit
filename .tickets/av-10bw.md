---
id: av-10bw
status: open
deps: [av-fw1b]
links: [av-2p8z]
created: 2026-08-18T05:57:15Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-1in5
tags: [hosted, backend, api, storage]
---
# Storage quotas: refuse writes that would exceed an owner's limit

Nothing limits how much an owner stores. On a self-hosted instance that is correct — it is the operator's disk and their library. On a hosted instance with open signups it is an unpriced liability: one person can vendor a few hundred megabytes of wasm payloads through URL snapshot ingest, which is a supported feature working exactly as designed.

This is the ticket that makes a paid tier mean something. Until a limit exists, every plan is the same plan.

## Design

**Enforce where bytes enter, which is more places than the upload form.** `POST /api/artifacts` is the obvious one, but a body-rewriting `PATCH`, `POST /api/artifacts/:id/refetch`, `PUT /api/artifacts/:id/widget` and the agent's `create_artifact`/`update_artifact`/`set_widget` tools all grow an owner's storage. The agent path matters most, because it is the one driven by model output rather than by a person clicking something.

**Check before the write, and check the projected total**, not the current one — an ingest that fits under the limit before it runs and exceeds it after has not been limited.

**Snapshot ingest is the case that needs care.** The vendorer fetches bounded by its own per-asset and total caps and produces a report; the size is not known until it has run. Refusing after a full vendor pass wastes the work but is honest, and it is better than the alternative of refusing based on a guess at the final size. If the pre-fetch estimate is available cheaply, use it as an early-out — but the authoritative check is on the assembled document.

**A refusal must say what happened.** Over-quota is `413` with the limit, the current usage, and the size of the thing refused. A bare failure on a snapshot the user waited thirty seconds for is the worst version of this feature.

**The limit comes from the plan** ([[av-2p8z]]), with a configured default for owners who have none — which is every owner on a self-hosted instance. **Unset means unlimited**, so a self-hoster who never configures a quota never meets one.

**Reads and deletes are never refused.** An over-quota owner can still open, export and delete their artifacts; the only thing that stops is adding more. A limit that locks someone out of their own library to punish them for filling it is a data-hostage, not a quota.

## Acceptance Criteria

- With no quota configured, no request is refused, on any path.
- With a quota configured, each of these refuses an over-limit write with `413` and leaves stored bytes unchanged: artifact create, body-rewriting `PATCH`, refetch, widget `PUT`, and each agent tool that writes.
- The check is against the projected total after the write, not the total before it.
- A refused snapshot ingest reports the limit, current usage, and the assembled size — never a bare failure.
- An over-quota owner can still list, open, render, export and delete their artifacts, and deleting brings them back under the limit.
- The limit resolves from the owner's plan when one exists, and from the configured default otherwise.

