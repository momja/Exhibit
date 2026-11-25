---
id: egt-rsxd
status: open
deps: []
links: [Exh-5hmx, Exh-g2vv]
created: 2026-11-25T07:53:43Z
type: bug
priority: 2
assignee: Max Omdal
---
# Clarify opening connected artifacts from the agent preview

The agent preview shows a disabled “Preview only” control with an external-open icon for artifacts with connected data. It looks like a broken Open button and does not clearly explain the available action. The server currently blocks direct top-level opening for protected artifacts; connected data requires the isolated Exhibit preview. Decide the intended behavior before implementing: whether to provide an isolated view in a new tab or clearly communicate the restriction and direct users to the artifact view. Preserve connected-data isolation and account protections. Consider consistency with the artifact detail view and accessibility.

## Acceptance Criteria

Document the intended behavior for connected artifacts and implement a clear, accessible agent-preview experience consistent with that decision. Verify the available action and connected-data protections, including after preview swaps. Reference the original agent direct-open story and the relay/connectors epic for context.

