---
id: Exh-3r39
status: closed
deps: []
links: []
created: 2026-10-24T21:20:06Z
type: task
priority: 2
assignee: Max Omdal
tags: [docs, onboarding, self-host, agent]
---
# Add a copy-paste agent setup prompt to the README

Add a short README prompt a person can paste into a coding agent to have it clone
Exhibit, read the existing setup documentation, install dependencies, deploy on
their machine, and help them get started.

## Scope

Keep the instructions to a couple of paragraphs in the README. Point the agent
to README.md and docs/deployment.md rather than adding new setup guides. Explain
that environment values should be entered locally in an editor or deployment
secret settings, never supplied through chat, and that saved configuration must
be loaded by the chosen deployment.

## Acceptance Criteria

- README has a self-contained, copyable prompt with the repository URL and paths
  to the existing setup documentation.
- The prompt asks the agent to perform setup, verify the deployment, and help the
  person start using Exhibit.
- The agent asks how and where to self-host (Docker Compose or from source)
  and waits for the person's choice before installing dependencies or deploying.
- Environment configuration happens locally, without asking for values in chat,
  and existing configuration/data and secrets are handled appropriately.
- Implementation is limited to the README and this ticket; no additional guides,
  deployment changes, or ignore rules.
