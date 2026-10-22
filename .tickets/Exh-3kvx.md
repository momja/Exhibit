---
id: Exh-3kvx
status: open
deps: []
links: []
created: 2026-10-22T08:16:35Z
type: task
priority: 3
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, validation]
---
# [Experimental] Validate live provider accounts and staging connector costs

The relay/connectors implementation includes TLS-backed OAuth, vendor, SimpleFIN, quota and revocation fixtures plus a live HTTPbin bearer example. This environment has no provider accounts, OAuth registrations, Nango staging account or model API credentials. Complete the external-account acceptance checks on an operator-configured staging instance.

## Acceptance Criteria

Connect a real SimpleFIN account and inspect frame/state/export/transcript secrecy; connect, refresh and revoke GitHub OAuth; complete a reviewed long-tail provider through Nango and verify idle deletion; measure actual monthly stored-connection/compute/egress costs per active user on staging; run one live model arXiv generation and confirm direct fetch without a public proxy or no-cors. Record evidence and dates without credentials.

