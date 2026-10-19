---
id: Exh-ruds
status: closed
deps: []
links: []
created: 2026-10-19T01:24:20Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, agent]
---
# Add extensible agent message actions with copy text

Add a standard action row directly below each agent response in live chat and conversation history, starting with Copy text. Keep message content separate from actions and provide a clear extension point for future capture or message actions.

## Acceptance Criteria

Agent text messages show an accessible Copy action below the message; copying preserves message text without action labels; success and failure are communicated; streaming and history/resume work; additional actions can be added through the same action row.


## Notes

**2026-10-19T01:30:28Z**

Implemented shared agentMessage template and extensible ES-module action registry. Copy preserves exact message text, reports success/failure, and supports HTTP via selection fallback. Works for streaming, htmx history, and resumed message shells. Validation: make assets; go test ./...; focused gallery/action and template tests; browser copy clicks on live and history, mobile overflow check, dark mode screenshots in docs/screenshots/Exh-ruds. Extension guide in docs/agent.md.

**2026-10-19T04:17:31Z**

Changed message actions to show icons only, with text tooltips on hover or keyboard focus and no layout shift. Updated desktop/mobile/dark-history screenshots and added hover screenshot. Gallery script/template suite and browser hover/focus verification passed.
