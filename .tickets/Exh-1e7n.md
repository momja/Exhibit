---
id: Exh-1e7n
status: closed
deps: []
links: []
created: 2026-10-30T06:41:43Z
type: feature
priority: 2
assignee: Max Omdal
---
# Render agent Markdown without HTML or message boxes

Support Markdown in assistant messages with a maintained standard parser. Never render raw HTML. Remove assistant message bubbles; keep person messages boxed. Cover live streams, saved history and resumed conversations.

## Acceptance Criteria

Markdown renders during streaming and in saved/resumed conversations. Raw HTML and unsafe links stay inert. Only person messages have bubbles. Desktop and narrow mobile edge cases verified with screenshots. Run standards/spec review agents, address findings and create a PR with screenshots.


## Notes

**2026-10-30T07:18:24Z**

Implemented server-rendered CommonMark/GFM with Goldmark v2.1.9, raw HTML disabled, unboxed agent/status messages, preserved Markdown Copy and contained mobile code/table scrolling. Standards and Spec reviews each found one P2 (per-delta snapshot amplification; blocked links reloading the page); both fixed in 7e37857 and fresh reviews report no remaining findings. Verified make build, full go test ./..., targeted Markdown race/browser/security tests, go vet, all 375 gallery Node tests, and all six Playwright journeys on isolated scratch ports. Seven before/after and mobile edge-case screenshots are included for the PR.
