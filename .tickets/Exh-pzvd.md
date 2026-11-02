---
id: Exh-pzvd
status: in_progress
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

