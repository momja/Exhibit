---
id: av-jafp
status: closed
deps: []
links: []
created: 2026-07-18T15:23:11Z
type: epic
priority: 2
assignee: Max Omdal
tags: [ui]
---
# Security posture UI: badge, popover, allowlist management

Umbrella for the capability/allowlist UI reworked in Paper (file 01KWQV7Y6G82HDZJ5CKPJTSJ7Y). Reframes an artifact's sandbox posture from a green/amber "verdict" into neutral, informational transparency (spec 6.2), and moves allowlist + capability *management* out of the viewer and onto the Edit page.

## Children

- av-xgik (chore) — shared gallery design tokens + component CSS. Foundation; the others build on its classes.
- av-isb3 — gallery card capability-row badge.
- av-41se — capability posture detail popover (card + viewer).
- av-p0a1 — Edit page: allowlist + capability management, responsive (desktop + mobile).
- av-hwx2 — viewer read-only posture + Manage link.

## Direction / decisions

- Neutral capability-row badge (retire green/amber); detail lives in a hover/focus popover.
- Viewer becomes read-only; allowlist + capabilities are managed on Edit (collapsible security panel; source stays CodeMirror).
- Capabilities use the EXISTING downloads_approved / clipboard_approved booleans (Ask first / Always allow) — no "Block", no schema change.
- Standard web components + a shared CSS layer over the server-rendered html/template stack; no frontend framework, no Tailwind (see av-xgik).

Mockups: each child ticket references its specific Paper artboard(s).
