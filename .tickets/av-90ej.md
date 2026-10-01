---
id: av-90ej
status: in_progress
deps: [av-c7hu]
links: []
created: 2026-10-01T03:13:09Z
type: bug
priority: 2
assignee: Max Omdal
tags: [ui, agent]
---
# Agent thinking spinner is static under Reduce Motion

On a phone with Reduce Motion enabled the thinking spinner does not turn, so an in-flight turn looks frozen. This is the `@media (prefers-reduced-motion:reduce)` override av-u5k7 added on purpose. Emulated iPhone WebKit and Chromium both spin with no preference and stop under reduce; nothing else in agent.css touches `.thinking` on narrow screens.

Decision (2026-09-30): the spinner keeps spinning under Reduce Motion. A 13px icon turning in place is not the sweeping motion the setting exists to suppress, and the iOS activity indicator keeps spinning with it on. This reverses av-u5k7's "no spin under reduce" criterion.

## Acceptance Criteria

- The thinking spinner rotates with prefers-reduced-motion: reduce, verified in emulated mobile WebKit.
- The CSS says why the indicator is exempt, so the override does not come back.

