---
id: Exh-f50x
status: open
deps: [Exh-numw]
links: []
created: 2026-07-11T05:02:43Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-avau
---
# Docs: static build command, manifest format, deploy workflow

Document the 'exhibit build' subcommand, the manifest format (Exh-un03), and a deploy walkthrough against at least one real static host (e.g. GitHub Pages) confirming the output directory works as-is with no extra server-side config. Explicitly document the read-only/no-sync limitation (state baked in at build time, no write-through, re-run the build to refresh) so it's not mistaken for a live, syncing deployment.

## Acceptance Criteria

docs/ includes a page covering: the build command and flags, the manifest file format with an example, a step-by-step deploy to a real static host verified to work, and an explicit note that the output is a point-in-time read-only snapshot refreshed by re-running the build.

