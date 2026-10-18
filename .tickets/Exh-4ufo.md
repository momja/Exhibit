---
id: Exh-4ufo
status: closed
deps: []
links: []
created: 2026-10-18T23:31:58Z
type: chore
priority: 2
assignee: Max Omdal
---
# Set up PR-Agent GitHub Action

Add The-PR-Agent/pr-agent to Exhibit as a GitHub Action with automatic pull request reviews, collaborator comment commands, pinned runtime, and API-secret setup documentation.

## Acceptance Criteria

Workflow reviews ready pull requests and updates; collaborator slash commands work; required secrets and rollout are documented; workflow passes actionlint.


## Notes

**2026-10-18T23:35:34Z**

Implemented an OpenAI-backed GitHub Action with a v0.48.0 Docker digest pin, non-draft PR and push reviews, collaborator-only slash commands, persistent review comments, and setup documentation. Validation: actionlint v1.7.12; TOML parsing and upstream setting-name checks; 20 event-routing cases with the GitHub Actions expression evaluator; concurrency isolation; missing/present-key preflight. Registry tag/digest mapping and upstream action entrypoint checked. Live activation still requires the OPENAI_KEY repository secret and merging the PR.
