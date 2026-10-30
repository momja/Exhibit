---
id: Exh-k6gt
status: closed
deps: []
links: []
created: 2026-10-30T05:00:45Z
type: feature
priority: 1
assignee: Max Omdal
parent: Exh-g2vv
---
# Make V1 connections personal and advanced; preserve setup and repair journeys

Required epic corrections: personal account access only; recipients connect their own accounts; saved data explained separately at sharing/export. Generic credential setup behind advanced experimental configuration and UI, no service catalog. Preserve tool context through setup/OAuth, actionable agent requests, and repair existing connections without replacing grants.

## Acceptance Criteria

Backend and migration enforce personal credential use; custom setup gated in configuration and UI; safe tool context survives OAuth; requests have a next action; reconnect preserves identity and dependent access; tests, docs, screenshots updated.


## Notes

**2026-10-30T05:46:45Z**

Implemented the required V1 corrections. Store/relay grants authorize only the viewer's own account; migration 050 withdraws historical delegation while preserving saved results and provenance across sharing-mode changes. Sharing/export disclosures distinguish saved results from account access. Generic setup and repair require CONNECTORS_CUSTOM_ENABLED=true and Advanced; normal setup uses enabled named services, without adding a provider catalog. Tool context survives setup/authentication; connection requests have setup/access/reconnect actions; repair preserves the existing identity, grants, schedules and quotas while clearing stale credential responses. Operator usage remains under Advanced, including with engines disabled.

Rebased onto origin/main 9b541f3 and integrated the shared app header. Updated API, architecture, security, product and operator docs plus six desktop/mobile/advanced/repair/share/consent screenshots. Validation passed: complete go test -json ./...; go vet ./...; make assets; all 439 gallery JS tests; real Chromium connection/header journeys; screenshot capture; upgrade/personal-access/reconnect/body-limit regressions. Live account/registered OAuth/vendor staging checks remain separately tracked in Exh-3kvx; no product decision is pending in this correction.
