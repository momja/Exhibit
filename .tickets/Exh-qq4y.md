---
id: Exh-qq4y
status: closed
deps: []
links: []
created: 2026-11-16T06:08:32Z
type: feature
priority: 2
assignee: Max Omdal
---
# Reusable inline copy field for MCP settings

Replace standalone Copy URL and Copy command buttons in Connected agents with an inline icon in a reusable text field component. Identify other copy locations without migrating them.

## Acceptance Criteria

Shared Go partial, CSS and Stimulus controller; endpoint and Claude command use it; accessible keyboard copy with success/error feedback; responsive browser verification.


## Notes

**2026-11-16T06:13:10Z**

Implemented shared copyField partial/view, shared CSS and Stimulus controller for endpoint and Claude Code fields. Other discovered copy controls (issued token, share URL, widget embed, agent messages) remain unchanged. Chromium verifies keyboard, literals, failures, replacement and six viewport/theme states; screenshots retained by browser CI artifact.
