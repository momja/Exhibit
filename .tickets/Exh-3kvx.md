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

The relay/connectors implementation includes TLS-backed native OAuth, Nango inject-mode, Composio forward-mode, generic Basic-auth, quota and revocation fixtures plus a live HTTPbin bearer example. This environment has no provider accounts, OAuth registrations, Nango or Composio staging credentials, or model API credentials. Complete the external-account acceptance checks on an operator-configured staging instance.

## Acceptance Criteria

Connect a real Basic-auth API through a generic connection by pasting a URL with embedded credentials, and inspect frame/state/export/transcript secrecy; connect, refresh and revoke GitHub OAuth; complete a reviewed long-tail provider through Nango and verify idle deletion; complete another through Composio and verify hosted consent, account ownership, forwarded status/headers/body, revoked/expired accounts and upstream revocation behavior; run both engines on one instance, switch a provider engine and verify old connections continue while new ones use the new engine; compare the operator's monthly stored-connection and call counters against actual vendor billing, compute and egress per active user; run one live model arXiv generation and confirm direct fetch without a public proxy or no-cors. Record evidence and dates without credentials.

## Fixture evidence and live checks still required (2026-10-23)

Composio's current official token-custody, Connect Link, proxy, revocation and retention documentation was checked before implementation. It redacts retrievable credentials, so the adapter uses explicit forward mode. TLS fixtures cover native/Nango/Composio policy parity, bound completion, same-instance mixed engines, engine changes, normalized errors, cost counters and response limits. These checks establish the implementation contract, not live provider compatibility.

On staging, verify the operator's actual Composio project/plan retention settings, especially Zero Data Retention coverage for managed versus custom OAuth apps. Inspect the grant disclosure with a real provider, confirm exact-origin refusal before handoff, and test provider-specific redirects under the documented forward-mode limitation. Vendor binary-download URLs are intentionally refused. No real Composio consent or bill has been validated in this environment.

## Notes

**2026-10-24T04:29:39Z**

2026-10-24 additive-permission validation: the existing native GitHub Bearer connection and the artifact's separately approved unpkg.com icon assets work together on the running demo, with GitHub relay status 200 and no connection-data notice. No origin/grant/state reset was performed. Two-connection, standalone API/CDN, schedules, legacy metadata and portable parity are fixture-tested, including races. This is native PAT validation; real Basic account, registered OAuth and Nango/Composio staging/retention/cost checks remain open.
