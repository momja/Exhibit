---
id: Exh-5kzm
status: closed
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


## Notes

**2026-11-27T08:03:24Z**

Implemented agent-proposed advanced API connectors on PR #315 typed interactions. Secret-free immutable proposals; private owner-only key/trust card; exact normalized HTTPS origin validation; known key/destination mismatch refusal; encrypted credentials; atomic duplicate prevention across proposal and manual native creation; sticky denials and safe disconnected outcomes. Stop retires approvals before Pi acknowledgement; failed replies reconcile to Continue without key recollection. integrations.sh endpoint/OpenAPI discovery investigated and documented as untrusted guidance with no runtime dependency. Opus security and Sol correctness PASS at implementation 0aa35373016e1cc23dfb2cac2f36e59f2bbe43ac; DS Flash UX PASS on e3eb1686 (subsequent changes were tests/docs only). Reports/screenshots outside checkout: http://t3code:18483/security-final.md, correctness-final.md, ux-final.md, connector-1280.png, connector-390.png (dark), connector-320.png. Focused API/route checks42.6s, saved-question regressions52.4s, mismatch retry browser10.1s,49 Node checks, extension/golden/service checks and account/store checks passed. Custody concurrent approval/manual-create and agent lifecycle tests passed under race detector. Broad short suite passed all completed packages but API reached default10m timeout during unrelated remaining tests; current full API rerun uses20m. Known limits: stopped durable proposals retain pending slots (cap8); private-input guarantee does not cover chat-pasted keys; manual native duplicates share owner/origin/header identity. Leave PR unmerged for human review.
