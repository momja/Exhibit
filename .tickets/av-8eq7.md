---
id: av-8eq7
status: in_progress
deps: []
links: []
created: 2026-10-01T02:46:27Z
type: chore
priority: 2
assignee: Max Omdal
tags: [docs, agent]
---
# Docs: present the agent as core, not an optional surface

Decision (2026-09-30): Exhibit commits fully to the agent interaction plane. Deploying without agent UI is not a supported mode, and we will not add a pi-optional build or hide agent UI when pi is missing. The docs still sell the agent as optional and tell operators how to strip it out.

Passages that frame the agent as optional:
- docs/deployment.md §4 heading 'AI agent (optional)'
- docs/deployment.md §4.3 'No AI agent features': tells operators to drop pi and swap the Dockerfile runtime stage. Remove the section.
- README.md 'Optional:' list: pi 'only for the AI agent surface; if absent, that surface disables itself and nothing else changes'. pi is a requirement for running the server outside Docker.
- docs/architecture.md §3.7 opening ('If the pi binary is absent the surface degrades to disabled; nothing else changes') and §9 ('absent entirely when pi is not installed').
- Dockerfile runtime-stage comment: 'TODO: install this conditionally. Don't install by default for all deploys', plus the closing 'stays safe to run without it configured' line.
- docker-compose.yml: 'Optional agent settings' comment. The settings are optional (BYO key is the default), the agent is not; reword so it does not read as the feature being optional.

Leave alone, because they describe behavior the code still has: the 503 'pi binary absent' responses in docs/api.md and docs/agent.md, PI_BIN in docs/agent.md's env table, the disabled Generate-widget button in docs/widgets.md. Docs describe what is built, so a missing pi is still documented. Describe it as a broken install the server tolerates, not as a configuration.

Out of scope: making a missing pi fatal at startup. That is a code change and gets its own ticket if wanted.

## Acceptance Criteria

- No doc or comment presents running without the agent as a supported deployment.
- deployment.md has no 'No AI agent features' section, and §4 is not labeled optional.
- README lists pi as required to run the server outside Docker.
- The Dockerfile TODO about conditional install is gone.
- Remaining mentions of a missing pi describe the fallback behavior accurately and frame it as a broken install.

