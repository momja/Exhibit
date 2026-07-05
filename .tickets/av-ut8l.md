---
id: av-ut8l
status: open
deps: []
links: []
created: 2026-07-05T17:29:12Z
type: chore
priority: 2
assignee: Max Omdal
---
# Set up golangci-lint for Go service

No Go linter is currently configured. Add golangci-lint to the project so Go code has consistent, automated static analysis.

## Acceptance Criteria

- .golangci.yml added at repo root with a sensible default set of linters (e.g. govet, staticcheck, errcheck, unused, gofmt/goimports, gosimple)
- 'lint' target added to Makefile (e.g. 'golangci-lint run ./...')
- Existing code passes lint cleanly (or issues are fixed/explicitly excluded with justification)
- Document how to run it in docs or README

