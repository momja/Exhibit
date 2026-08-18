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
# Polar subscription state: plan on the user, maintained by webhook

Payments land on Polar — a merchant of record, so international VAT/GST registration and remittance are theirs rather than ours, which is the compliance burden that actually costs a small team. But Polar knows what someone paid for and Exhibit knows what they may do, and nothing joins the two.

This is that join, and it is deliberately small: a plan on the user, and a webhook that keeps it current. No billing provider reaches into `internal/api` to decide whether an ingest is allowed — the gates ([[av-10bw]], and later the agent's) read a column, and this ticket is what maintains it.

## Design

**Plan state lives on `users`, and the gates read only that.** Not a Polar API call on the request path — that is the same mistake as verifying a provider-signed token per request (`architecture.md` §3.8): it puts a network dependency in front of every gated action and makes an outage at the payment provider an outage of the product. An owner whose plan is already recorded keeps working while Polar is unreachable.

**The webhook is the single write path for it**, consistent with the rest of the system. Signature-verified, idempotent (providers retry, and a replayed upgrade must not double-apply), and tolerant of out-of-order delivery — a stale event must not downgrade an account that a newer event already upgraded, so order by the event's own timestamp rather than by arrival.

**Route placement.** It is unauthenticated in the session sense but authenticated by signature, so it belongs outside the authenticated API group, alongside `/api/settings/public` as the second deliberate exception — and like that one, registered only when the hosted configuration is present. On a self-hosted instance the route does not exist.

**Absent plan means the configured default.** Every self-hosted owner has no plan and must behave exactly as today; the default is what [[av-10bw]] resolves its limit from, and unset is unlimited.

**Reconciliation, because webhooks are lossy.** A missed or dropped event leaves an account on the wrong plan indefinitely, and the user experiencing it has paid. A path that re-reads subscription state for one owner and repairs the column is the answer, callable by an admin at minimum.

**Deliberately not here: usage-based billing.** Polar supports metered billing and the agent will want it, but there is nothing to meter yet — that is [[av-hyo6]]. This ticket handles subscription state only.

## Acceptance Criteria

- `users` carries plan and subscription status, and every gate resolves an owner's entitlement from it with no outbound call.
- A signature-verified webhook creates, upgrades, downgrades and cancels correctly; an invalid signature is rejected.
- Replaying an event is a no-op, and an out-of-order event does not overwrite newer state.
- With the hosted configuration absent the route is not registered, every owner resolves to the default plan, and behavior is identical to today.
- A reconcile path repairs one owner's plan from the provider's current state.
- Payment-provider unavailability does not block any gated action for an owner whose plan is already recorded.

