---
id: Exh-ghuv
status: in_progress
deps: []
links: []
created: 2026-10-18T16:55:44Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, detail]
---
# Collapse the artifact detail page's two-row header into compact chrome

On desktop the artifact detail page (/artifacts/:id) stacks two full-width bars above the sandboxed frame: the app header (back link, title, created date) and a toolbar row (Open in new tab, Edit, Modify with agent, Download file, sandbox posture cluster, Manage in allowlist settings, Share, Update from source). Together they take ~95px of vertical space on a page whose primary interface is the artifact itself. Mobile already solved this with a one-row header plus a bottom-sheet toolbar (av-g7n7); desktop still has the two rows.

## Design

Propose five interface options on the paper canvas, pick one, implement it in detail.tmpl/detail.css/detail.js. The toolbar DOM must stay the single definition of the action list (the mobile sheet restyles the same markup). Owner-only controls stay gated on .ReadOnly; the capability cluster stays an htmx target (av-mf1x).

## Acceptance Criteria

Desktop detail page chrome is a single bar no taller than --header-height (40px). Every existing action remains reachable for an owner, and a recipient sees the same reduced set as today. Mobile bottom-sheet behaviour unchanged. Page-script tests and Go template tests pass. Screenshots in docs updated.

