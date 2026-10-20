---
id: Exh-jtv1
status: open
deps: [av-f05n]
links: []
created: 2026-10-20T06:22:17Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-f3cp
tags: [testing, e2e, frontend]
---
# e2e: expand ingest, sharing, search, and tag flow coverage

Preserve the wider UI scope formerly grouped into [av-f05n](av-f05n.md), after the first modal/htmx journey proves the [av-f3cp](av-f3cp.md) harness and CI workflow. Use current product behavior and expand in small specs; API helpers seed prerequisites, while the behavior under test is exercised through the real UI.

## Coverage

- **Ingest approval:** paste a local fixture document containing two known external origins through the current new-artifact UI. Assert the approval surface lists the detected origins, approve a subset, and verify the persisted allowlist and rendered artifact. Use test-owned endpoints and deterministic fixture content rather than a live CDN.
- **Sharing:** use the existing owner sharing panel to enable/copy an anonymous link. Open it in a clean context without owner credentials and assert actual artifact rendering. Revoke or replace the link through the owner UI and require the old URL to stop rendering. Shares have no expiry; `expires_at` is rejected, so do not retain the old expiry scenario or API-only minting as the flagship UI path. Distinguish app wrapper and render-surface URLs.
- **Search and tags:** seed distinct artifacts, exercise eager search with its actual htmx grid swap, and assert matching cards. Add/attach, rename/recolor, detach, and globally delete tags through their existing UI; verify persistence and propagation to all affected cards, including after replacement and reload.
- **Later bridge/edit gaps:** retain download approval, clipboard approval/bridge, allowlist edits, and refetch from the historical scope. Inventory existing coverage first and record any unimplemented browser journeys as separate linked gaps rather than silently treating them as covered. Use deterministic clipboard/download fixtures and local refetch sources; no provider/agent workflow is required.

Use accessible locators, observable API/DOM conditions, fresh owner/anonymous contexts as appropriate, and failure traces. Do not encode the obsolete assumption that every gallery page must be a classic script; modules, legacy bootstrap values, and Stimulus must work together according to current page contracts.

## Acceptance Criteria

1. Ingest approval persists exactly the selected origins and the artifact works through its app iframe.
2. The sharing panel yields a working credential-free link; revocation/replacement invalidates the old link. No expiry behavior is invented.
3. Search/tag mutations update the actual server-rendered gallery without a manual full reload, persist after reload, and leave replaced components usable. Shared tags update every affected artifact.
4. The implemented journeys run in the required hosted suite. Remaining bridge/edit journeys have explicit linked follow-ups with behavior-based criteria before this story closes.
