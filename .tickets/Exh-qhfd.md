---
id: Exh-qhfd
status: closed
deps: [Exh-spg4]
links: []
created: 2026-10-22T06:50:51Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, vendor]
---
# [Experimental] Phase 4: vendor credential engine (Nango) for long-tail OAuth

EXPERIMENTAL; part of Exh-g2vv.

- **Engine:** a second credential engine behind the phase-2 interface. The relay maps an Exhibit connection to a Nango connection id and uses Nango's credential-injecting proxy; Exhibit keeps the boundary (allowlist, grants, quotas, logs).
- **Facts checked 2026-10-22:**
  - Nango Pay-as-you-go: $0.29 per stored connection per month, prorated, charged whether used or not; $50/month base with $50 of credits; free plan capped at 10 connections. Proxy requests aren't metered separately; compute ($0.72/hour) and egress ($0.50/GB) are.
  - Free self-hosted edition: Auth + Proxy only. Docker image plus Postgres and Redis.
  - License: Elastic License 2.0. Not OSI open source; no offering as a hosted or managed service; no bypassing license keys. Bundling self-hosted Nango into a hosted Exhibit needs legal review or a commercial agreement; Nango Cloud otherwise.
  - Alternatives: Pipedream Connect (hosted, managed OAuth clients, proxy API); Paragon (hosted, proxy API).
- **Cost control:** prune vendor connections unused for N days, asking the user to reconnect next time; vendor connections count against the small entitlement from phase 3.

## Acceptance Criteria

- A long-tail provider connects through Nango without the artifact or the agent changing.
- Idle pruning removes connections at the vendor.
- Cost per user is measured on staging.


## Notes

**2026-10-22T16:01:53Z**

Implemented managed Nango Connect UI with browser-bound server polling, provider/owner/flow verification, separate entitlement cap, revocation and idle vendor deletion. Deliberate security deviation: retrieve managed credentials and use the single Exhibit egress gate instead of the vendor proxy, so every redirect is checked and reflected credentials can be scrubbed. TLS protocol fixtures pass. Live provider consent and actual staging cost measurement require an operator account and are tracked in Exh-3kvx.
