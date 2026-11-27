---
id: Exh-5kzm
status: in_progress
deps: []
links: []
created: 2026-11-27T07:27:28Z
type: feature
priority: 1
assignee: Max Omdal
---
# Agent-proposed advanced API connectors with private key approval

Build on PR #315 typed interactions. An agent proposes a validated, secret-free API connector stub; an owner privately enters an API key and explicitly trusts the exact destination before any connector is created. Never expose credentials to agents. Prevent duplicates and handle denial gracefully. Explore integrations.sh for endpoint/schema discovery. Verify with opus, sol and ds-flash security/UX/correctness panel.

## Acceptance Criteria

Validated HTTPS endpoint/auth fields; explicit domain trust and private key input; owner-only approval; no secrets in interactions, tools, history or logs; atomic duplicate prevention; denial/cancellation safe; integration discovery guidance; regression tests and browser evidence.

