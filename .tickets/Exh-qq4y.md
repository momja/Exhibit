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

Replace standalone copy buttons with an inline icon in a reusable text field component. Apply consistently to MCP endpoint, Claude Code command, newly issued MCP token, public share link and widget embed code.

## Acceptance Criteria

Shared Go partial, CSS and Stimulus controller used by all five existing copyable text fields; accessible keyboard copy with success/error feedback; responsive browser verification; token cleanup and sharing refresh/locking preserved.


## Notes

**2026-11-16T06:13:10Z**

Implemented shared copyField partial/view, shared CSS and Stimulus controller for endpoint and Claude Code fields. Other discovered copy controls (issued token, share URL, widget embed, agent messages) remain unchanged. Chromium verifies keyboard, literals, failures, replacement and six viewport/theme states; screenshots saved under /tmp/exhibit-review/Exh-qq4y and linked from the PR through the Tailscale evidence server (CI artifact upload was unavailable with the configured GitHub workflow permissions).

**2026-11-16T06:40:45Z**

User expanded scope to all obvious copy fields. Migrated public share link, widget embed code and newly issued MCP token; removed their duplicated clipboard handlers and obsolete CSS. Real Chromium coverage now includes token issuance/copy/cleanup, sharing link replacement via htmx, native pending fieldset locking, widget literal bytes and manual fallback. Agent message copying is a message action rather than a text field and remains distinct.
