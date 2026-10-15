---
id: av-xie4
status: in_progress
deps: []
links: []
created: 2026-10-15T07:32:05Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, agent]
---
# Dark mode that follows the system color scheme

The app is light-only. Make every app-origin page follow the OS setting through prefers-color-scheme, with no in-app toggle. Also teach the agent to build artifacts and widgets that do the same, since a widget renders on a transparent background inside a gallery card and will be unreadable on a dark card if it hardcodes dark text.

Scope:
1. App UI: add a dark palette to web/gallery/tokens.css under @media (prefers-color-scheme: dark), and declare color-scheme: light dark so native controls, scrollbars and form fields follow. Audit every web/gallery/*.css and template for literal colors that bypass the tokens and route them through tokens. Covers gallery, detail, edit, new, agent, login, settings/profile/admin, 404, and the default card tile.
2. CodeMirror: the editor bundle (web/editor) picks a dark theme when the system is dark, and switches live when the setting changes.
3. Logo: check the server-rendered SVG logo (internal/color/brand.go, internal/api/logo.go) stays legible on a dark header.
4. Agent prompt (internal/agent/prompt.go): add instructions that generated artifacts AND widgets support both light and dark via prefers-color-scheme, with CSS custom properties and color-scheme: light dark. Widgets must keep a transparent background and pick text colors that read on both a light and a dark card. Update prompt_test.go to pin the new text.
5. Docs: note the convention in docs/technical_stack.md §9 (only what is built).

## Design

Tokens first: page sheets should already read from tokens.css, so most of the work is a second set of values in one media block. Do not touch the render origin: an artifact is a visitor-authored file and owns its own colors. Iframe gotcha: a sandboxed frame whose color-scheme differs from its embedder's gets an opaque canvas background instead of a transparent one, so check that widget tiles and the artifact frame look right in both modes.

## Acceptance Criteria

- With the OS in dark mode, every app-origin page renders dark with readable contrast; with it in light mode nothing changes visually.
- Switching the OS setting updates open pages without reload (CSS and CodeMirror).
- Agent system prompt tells the model to support light and dark in artifacts and widgets; prompt_test pins it.
- go test ./... passes, including the page-script suite.
- Screenshots of key pages in both modes for the PR description (kept out of git).

