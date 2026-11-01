---
id: Exh-du7v
status: closed
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


## Notes

**2026-10-24T00:01:10Z**

Implemented the trusted runtime distinction between origin approval and connection data restrictions. The render preamble marks approved destinations excluded by the effective connection policy. The viewer and agent preview request an origin-specific server partial before showing actions; custody blocks show saved-approval status, an explanation, Close, and the artifact's connection-settings link. Origin allows now recheck active or retained custody and return 409 connection_data_policy without mutation when blocked; stale prompts refresh the explanation without a reload. Policy outages fail closed, shared viewers receive read-only explanations, and ordinary origin approval and redirect diagnostics remain intact.

Validation passed: all 391 gallery tests with NODE_OPTIONS=--experimental-vm-modules, the full internal/render suite, network/origin/relay/connection/private-render API regressions, focused TestNetworkOrigin race tests (including real Chromium), go vet for internal/api and internal/render, and git diff --check. The case-insensitive retired-connector grep remains empty. Chromium drives the real renderer, CSP, htmx partial and CSS in both viewer and agent preview, confirms no blocked stylesheet request escapes, and verifies ordinary approval still loads the stylesheet.

Updated docs/relay-connectors.md, architecture.md and security.md. Desktop/mobile evidence is in docs/screenshots/Exh-du7v/. Rebuilt and restarted the running experimental demo, then confirmed the reported artifact shows Connection data protection and Your origin approval is saved. Its unpkg.com approval remains allow, the connection grant still has no additional destinations, and retained data remains restricted to api.github.com. No credential, grant, source or saved-data change was needed. The CDN remains intentionally blocked; users can embed its assets or explicitly review data destinations in Connections.

**2026-10-24T04:12:45Z**

Superseded by the user's additive-connector clarification (2026-10-24, Exh-s47o). The global response-data ceiling was the wrong product behavior. Remove the connection-data-block notice and the API approval guard, retain ordinary origin approvals, and verify that multiple independent connectors and approved standalone/CDN traffic work together. The replacement fix preserves saved data and credentials; the prior explanatory screenshots are obsolete.
