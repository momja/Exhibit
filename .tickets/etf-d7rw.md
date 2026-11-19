---
id: etf-d7rw
status: open
deps: []
links: []
created: 2026-11-17T06:01:13Z
type: bug
priority: 1
assignee: Max Omdal
tags: [security, oauth]
---
# Review OAuth consent policy when embedded router login is disabled

Independent PR293 review reproduced pre-existing owner grant issuance through unauthenticated OAuth consent in an embedded api.Config with MCPURL and login disabled (including token-only configuration). mayManageAgentGrants allows PrincipalNone when loginEnabled is false. PR head bcbbfae50c989caf9cd4c823641f469253e6026e and baseline c93ed47b both issue an authorization code; the PR probe exchanges it for an owner agent token and reads artifacts. The default shipped server seeds local admin login and was not found reachable through this configuration. Evidence: /tmp/exhibit-review/Exh-lmor/pi-review-r3.md and pi-review-r3-probes; https://github.com/momja/Exhibit/pull/293#issuecomment-6940136054. This ticket records a finding, not an accepted authentication policy change.

## Acceptance Criteria

Choose and document the supported embedded no-login OAuth policy; if consent must require credentials, enforce it without breaking authenticated owner or agent-denial paths. Add regression coverage for login-enabled unauthenticated refusal, token-only no-login consent, and the consent-code-token chain. Verify shipped-server defaults and whether nondefault configurations can expose the condition. Preserve fail-closed legacy render-token refusal.

