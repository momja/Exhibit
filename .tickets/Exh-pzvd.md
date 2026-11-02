---
id: Exh-pzvd
status: closed
deps: []
links: []
created: 2026-11-02T01:40:32Z
type: bug
priority: 2
assignee: Max Omdal
---
# Keep agent placeholder on one line at narrow widths

Agent composer placeholder wraps and clips on phones with large fonts and resized desktop panels, especially with Stop visible. Change placeholder to Ask Exhibit... and prevent wrapping without changing multiline message behavior.

## Acceptance Criteria

Placeholder stays on one line at narrow panel widths and large font sizes, including while streaming; all composer states read Ask Exhibit...; typed multiline messages still grow and wrap.


## Notes

**2026-11-02T01:45:49Z**

Implemented a stable Ask Exhibit... placeholder in the server template and removed JS overrides. The empty field stays on one line with ellipsis, prevents horizontal scrollbars, and grows its minimum line height with larger fonts; entered messages retain normal wrapping and Shift+Enter. Regression reproduced with original CSS even with the shortened text (78px scroll height in a 38px field at desktop 280px/32px font). Validation: make assets; 113 agent JavaScript tests passed; focused agent page/versioned-assets Go checks passed; TestAgentComposerBrowser passed across build/modify, 320/390px phones and 280px desktop panel, 16/24/32px fonts, streaming, questions, settled state, and multiline typing. Six screenshots saved in docs/screenshots/Exh-pzvd.
