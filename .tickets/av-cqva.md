---
id: av-cqva
status: open
deps: []
links: []
created: 2026-07-05T17:29:23Z
type: chore
priority: 2
assignee: Max Omdal
---
# Set up ESLint for editor JS (web/editor)

No JS linter is currently configured for web/editor (the CodeMirror bundling island). Add ESLint so the JS source has consistent, automated static analysis before esbuild bundling.

## Acceptance Criteria

- ESLint config added under web/editor (flat config, eslint.config.js)
- 'lint' script added to web/editor/package.json
- Existing editor.js source passes lint cleanly (or issues fixed)
- Document how to run it (README or docs)

