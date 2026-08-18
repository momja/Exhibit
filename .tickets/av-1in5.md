---
id: av-1in5
status: open
deps: []
links: []
created: 2026-08-18T05:55:46Z
type: epic
priority: 2
assignee: Max Omdal
tags: [hosted, backend]
---
# Epic: Hosted Exhibit — the prerequisites for running it as a service

Exhibit is getting a hosted version. Self-hosting stays supported and stays the default — every ticket under this epic must leave the self-hosted instance byte-identical when its new configuration is unset, in the `OIDC_ISSUER` shape where absent means the feature does not exist.

What the hosted version needs that the product does not have is mostly *not* auth. That part is largely done: `UpsertUser` (`internal/store/sqlite_users.go:24`) creates the row on first login, so an OIDC provider gives self-registration, email verification, password reset and MFA with no code change, and owner scoping is already a real query predicate (av-ep8k). Payments land on Polar — a merchant of record, so international VAT/GST is theirs rather than ours, with usage billing available for the agent later.

What is actually missing is everything that makes a *tenant* mean something: blobs that live somewhere other than one machine's disk, a number for how much an owner is holding, a limit on that number, and a plan to read the limit from. This epic is those four, plus the platform credential the agent needs when the instance rather than the user pays for it.

Deliberately **not** here: agent token metering and spend caps. They are their own epic ([[av-hyo6]]) because they measure a different thing — what an owner consumes over time, rather than what they hold — and because the agent's spend problem is not solved by billing at all.

