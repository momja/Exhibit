---
id: Exh-ow1g
status: open
deps: []
links: []
created: 2026-10-23T05:21:31Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors]
---
# [Experimental] Remove the built-in provider-specific connector; generic Basic covers it

EXPERIMENTAL; part of Exh-g2vv. Decision (2026-10-23): Exhibit ships no provider-specific connector for that service; it must not be an official connector. The branch feature/Exh-g2vv/relay-connectors ships one as recipe id `simplefin`. Remove it and every reference to it.

## Remove
- The built-in recipe and its claim flow (setup-token decode and server-side claim): internal/connectors/connectors.go, plus provider-specific stand-in handling in internal/connectors/standins.go.
- Connect-page UI and copy: internal/api/templates/connections.tmpl.
- Example app, seed data and README entries: examples/connectors/ (the budget example and its references in seed.py and README.md).
- Docs: docs/relay-connectors.md and docs/api.md.
- Bundle runner references: internal/bundle/server.go, bundle.go, files/serve.mjs.
- Tests that name it, rewritten against the generic connector where they cover real behavior (quota, caching, secrecy, account deletion): internal/connectors/connectors_test.go, internal/store/connections_test.go, internal/store/connection_requests_test.go, internal/store/sqlite_account_test.go, internal/api/agent_connections_test.go, internal/api/relay_browser_test.go, internal/api/network_warnings_test.go, internal/api/pageowner_test.go, internal/bundle/bundle_test.go, internal/bundle/node_test.go, internal/agent/artifact_saved_test.go, internal/agent/ext/exhibit.test.ts.

## Generic replacement (the capabilities the built-in provided)
- **Basic connection accepts a pasted URL with embedded credentials** (https://user:pass@host/path), split into origin, Basic credentials and base path. The secret is stored encrypted and never shown again.
- **Optional per-connection limits set by the user:** at most N requests per day, and reuse responses for M minutes. Enforced by the relay across every artifact and device using the connection. This protects APIs that disable tokens exceeding a daily budget.
- **Every connection defaults to "data may go nowhere else"**, widened only by explicit approval. This replaces the built-in's financial-sensitivity default.
- **No one-time-token claim step.** Users redeem any setup token themselves and paste the resulting URL. Never let an artifact perform a claim: a one-time token is itself a credential.

## Migration
Existing connections created from the built-in recipe (experimental, so only staging or dev data) are converted to generic Basic connections with the same origin and credentials and default limits, or deleted with a reconnect prompt; pick one and note it.

## Acceptance Criteria

- `git grep -i simplefin` on the branch returns nothing outside history.
- A Basic connection created from a pasted credential URL authenticates relay requests to its origin only.
- Per-connection daily-request and cache limits are enforced across artifacts; exceeding the limit is refused before the provider is contacted.
- New connections default to no other destinations.
- All replaced tests pass against the generic connector.

