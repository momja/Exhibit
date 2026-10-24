---
id: Exh-du7v
status: in_progress
deps: []
links: []
created: 2026-10-23T23:47:44Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, permissions]
---
# [Experimental] Explain connection data restrictions without repeat origin approval

An artifact already allowed to load unpkg.com repeatedly shows Allow network access after a GitHub connection is granted with source-only data destinations. Its ordinary origin approval is saved, but connection data custody narrows CSP to api.github.com. The render reporter mistakes this separate restriction for a missing origin approval, and Allow reloads into the same prompt. Distinguish connection data policy from origin approval, retain the restriction, show an accurate trusted notice with a link to connection settings, and stop ineffective runtime approval/reload loops.

## Acceptance Criteria

An approved CDN blocked by active or retained connection data policy shows a connection-data explanation without an origin approval prompt. New origins outside that data policy also cannot enter an approval loop. Ordinary undecided origins that the data policy permits retain the normal approval flow; redirect diagnostics remain correct. Rendering and API checks stay fail closed, viewers cannot see another account policy, and no data destination is widened implicitly. Cover owner, recipient/agent and stale-policy cases with focused tests and update docs/screenshots.

