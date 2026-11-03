---
id: Exh-uqdc
status: open
deps: []
links: []
created: 2026-11-02T15:59:54Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-qkl7
tags: [security, connectors]
---
# Strict CSP on the app origin and isolate credential entry

App-origin pages set no script-src CSP; the only CSP headers are frame-src policies. /connections, where users paste API keys, does not load ClerkJS itself, but every other authenticated page does, and a same-origin page can script /connections through window.open or an iframe. One XSS or one compromised third-party script anywhere on the app origin can read a key as it is typed.

## Design

- A strict CSP for app-origin pages: script-src 'self' plus the Clerk Frontend API origin only where hosted login is configured, no unsafe-inline (move the inline bootstrap scripts to nonces or data attributes), object-src 'none', base-uri 'none'. Roll out with Report-Only first; nothing is reported to a server (product doc §9), so verify in browser tests.
- Trusted Types where the page scripts allow it.
- frame-ancestors 'none' on /connections and the reconnect pages.
- Record, not build: a separate credential-entry origin with no third-party script, fed by a single-use ticket from the app, is the only real boundary against same-origin script. File it as follow-up if the CSP leaves a gap.

## Acceptance Criteria

- Every app-origin page carries the CSP; a test fails on a template with an inline script lacking a nonce.
- Browser suites (gallery, agent, edit, connections, Clerk login) pass under the enforced policy.
- docs/security.md and docs/technical_stack.md §9 state the policy and the rule for new scripts.

