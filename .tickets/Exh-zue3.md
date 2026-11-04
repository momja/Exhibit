---
id: Exh-zue3
status: open
deps: []
links: []
created: 2026-11-04T05:36:21Z
type: epic
priority: 2
assignee: Max Omdal
tags: [inference, agent, security, experimental]
---
# Platform inference for artifacts: LLM and decision model calls without managing keys

An artifact can call a language model, or a fast decision model, from ordinary JavaScript. The person running it doesn't need a key, a provider account or any API knowledge. Exhibit makes the call on the server, with the same provider configuration the built-in agent uses, and charges it to the person viewing the artifact.

This is a capability bridge (PRD §6.3), not a connector. The frame never reaches the network. The render preamble installs window.exhibit.ai, the call is posted to the host frame like a state write, and the host makes the authenticated request to the single write path. The CSP and the artifact's origin allowlist are untouched.

Product decisions (2026-11-04):

- Two kinds of call. complete() is a general LLM call. decide() is a decision model: a fast, "system one" model that always returns schema-validated structured output.
- No inference on public links or for anonymous visitors, ever. No owner-sponsored links.
- Who pays: the viewer. On an artifact shared with named people, each recipient's calls spend that recipient's own allowance (or their own key), never the owner's.
- Providers follow the agent pattern: platform mode uses the instance credential (OpenRouter on the hosted version); self-hosters may configure any supported provider, and BYOK uses the viewer's own stored key.
- Accounting is separate from the agent's (its own ledger tag, reported separately), but it counts toward the same total usage limits.

Out of scope here: streaming, scheduled inference into saved state for widgets, tool use or multi-step agents inside artifacts, and in-browser local models.

