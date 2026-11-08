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

An artifact can call a language model, or a decision model, from ordinary JavaScript. The person running it doesn't need a key, a provider account or any API knowledge. Exhibit makes the call on the server and charges it to the person viewing the artifact.

This is a capability bridge (PRD §6.3), not a connector. The frame never reaches the network. The render preamble installs window.exhibit.ai, the call is posted to the host frame like a state write, and the host makes the authenticated request to the single write path. The CSP and the artifact's origin allowlist are untouched.

Product decisions (2026-11-04):

- Two kinds of call, implemented as separate provider families. complete() is a general LLM call. decide() calls a decision model (a "System One" model such as TypeSafe's Jev): it answers typed questions (yes/no, choice, rating, probability) with a confidence, and never generates text.
- One shared core and one bridge: who pays, whether they may, metering and limits live in one layer (Exh-hwjg), and the person is asked once whether a tool may use AI, never which kind of model.
- No inference on public links or for anonymous visitors, ever. No owner-sponsored links.
- Who pays: the viewer. On an artifact shared with named people, each recipient's calls spend that recipient's own allowance (or their own key), never the owner's.
- Providers follow the agent pattern: platform mode uses an instance credential, self-hosters may configure any supported provider, and BYOK uses the viewer's own stored key. LLM calls use the agent's providers (OpenRouter on the hosted version). Decision calls use decision-model APIs configured separately (TypeSafe on the hosted version, any System One–compatible endpoint for self-hosters).
- Accounting is separate from the agent's (its own ledger tag, reported separately), but it counts toward the same total usage limits.

Out of scope here: streaming, scheduled inference into saved state for widgets, tool use or multi-step agents inside artifacts, and in-browser local models.

