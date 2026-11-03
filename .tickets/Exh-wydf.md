---
id: Exh-wydf
status: open
deps: [Exh-68sx]
links: []
created: 2026-11-02T15:59:23Z
type: chore
priority: 2
assignee: Max Omdal
parent: Exh-qkl7
tags: [connectors, security, hosted, docs]
---
# Document hosted KMS and Fly hardening runbook

An AWS KMS key cannot be exported, so with Exh-68sx the threat moves from stealing a key file to gaining permission to use the key. Most of that protection is AWS and Fly configuration, not code. Write it down as a runbook for the hosted platform.

## Design

Cover, in docs/deployment.md (or a linked hosted-operations doc):
- Fly as the real perimeter: SSO/2FA for every org member, a small org, deploy tokens scoped to the one app (never org-wide tokens in CI), and who can fly ssh console.
- IAM role trust limited to the Fly org and the Exhibit app via the OIDC subject claim.
- A dedicated AWS account holding only this key; root user with hardware MFA and no access keys; humans through IAM Identity Center with MFA; no IAM users with long-lived keys.
- An explicit key policy: the app role may only generate/decrypt data keys with Exhibit's encryption context; a break-glass admin role (MFA) administers but cannot decrypt; no account-wide delegation, while never leaving the key without an admin.
- CloudTrail/EventBridge alerts on PutKeyPolicy, CreateGrant, DisableKey, ScheduleKeyDeletion and decrypt-volume spikes.
- A 30-day deletion window, optionally an SCP denying deletion; what losing the key means (users reconnect, nothing leaks).
- Why not to import key material.

## Acceptance Criteria

Runbook merged and linked from docs/deployment.md and docs/security.md; every item above is either a concrete step or explicitly marked optional.

