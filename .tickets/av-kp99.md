---
id: av-kp99
status: open
deps: []
links: []
created: 2026-10-01T04:28:50Z
type: chore
priority: 2
assignee: Max Omdal
tags: [docs, ui]
---
# Docs: ES modules for page scripts, htmx swaps as the default update path

Agents write most of this codebase and follow the documented conventions. Those conventions currently steer new frontend code toward classic scripts that share globals (so a page piles into one file, e.g. detail.js at 847 lines and agent.js at 759) and treat htmx swaps as an exception for cases where a reload would cost too much.

Change the documented process, not the code: new and substantially rewritten page scripts are native ES modules, and a change that alters what a region shows is re-rendered on the server and swapped with htmx. Existing code converts as agents touch it.

## Design

AGENTS.md UI Conventions: two bullets pointing at technical_stack.md §9.
technical_stack.md §9: a "Page scripts are ES modules" block (import by relative path, no bundler, build still copies verbatim; shared scripts convert last because a file with `export` cannot load as a classic script; testdom.mjs cannot resolve imports, so the first module that needs a page-script test extends it). Reframe the htmx block as the default, with rules for event-driven triggers, binding by delegation, and what stays client-side.
architecture.md §3.5: the htmx paragraph describes swaps as the default rather than the exception.

Verified before writing: node --check (Node 23) accepts module syntax, http.FileServer serves .js as text/javascript, and the app origin sends no CSP that would block modules.

## Acceptance Criteria

- AGENTS.md, technical_stack.md §9 and architecture.md §3.5 agree on both conventions.
- Every claim about the current stack stays true of the code as built.
- No code changes.

