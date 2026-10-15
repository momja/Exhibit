---
id: av-xgzv
status: open
deps: []
links: []
created: 2026-10-15T06:29:14Z
type: chore
priority: 2
assignee: Max Omdal
---
# Remove leftover beads issue-tracker files and ignore rules

Beads has been replaced by the tk ticket CLI, but the repo still carries its leftovers: the root issues.jsonl export (every issue in it already exists in .tickets/) and the .dolt/ and .beads-credential-key entries in .gitignore and .dockerignore, under a comment that says the block was added by bd init. Remove them. Closed tickets that mention beads are history and stay as they are.

## Acceptance Criteria

issues.jsonl is deleted. .gitignore and .dockerignore no longer list .dolt/ or .beads-credential-key, and the rules in that block that are still needed (*.db, data/, server) are kept under an accurate comment. No tracked file outside .tickets/ mentions beads, bd or Dolt. go build ./... and go test ./... pass.

