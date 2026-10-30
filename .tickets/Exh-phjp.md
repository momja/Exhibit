---
id: Exh-phjp
status: closed
deps: []
links: []
created: 2026-10-25T15:10:59Z
type: feature
priority: 2
assignee: Max Omdal
---
# Configure the agent thinking baseline and BYOK preference

Configure AGENT_THINKING_LEVEL as the instance baseline for Exhibit Pi sessions. Hosted/platform mode uses only this setting. In non-hosted BYOK mode, expose a per-owner override on the page where provider, model, and API key are configured. Cover chat, widget generation, and resumed conversations.

## Design

Validate the environment baseline at startup, defaulting to medium. Store an optional thinking level alongside each owner's API key; an empty preference inherits the baseline. Resolve personal preference above the baseline only in BYOK mode and pass the resolved level through Pi --thinking, including on resume. Keep hosted settings UI and API absent and ignore any saved personal preference. Existing keys migrate to inheritance without altering their ciphertext.

## Acceptance Criteria

Supported levels are off, minimal, low, medium, high, xhigh, and max. Invalid environment values fail startup and invalid preferences return 400. BYOK owners can save, reload, and clear preferences without re-entering a masked API key. Hosted/platform sessions use only the baseline and cannot change thinking through the interface or request body. Resumed sessions use the current resolved setting. Cover migration, persistence, precedence, and browser behavior; update documentation and screenshots.


## Notes

**2026-10-25T15:19:26Z**

Implemented AGENT_THINKING_LEVEL startup validation, a shared Pi --thinking override for fresh/resumed sessions, Compose forwarding, and deployment/agent documentation. Unset preserves existing defaults and saved thinking levels. Verified with real Pi RPC state tests, make assets, go test ./internal/agent ./internal/api ./cmd/server -timeout=10m, a built-server invalid-setting startup check, and Compose unset/high/off checks.

**2026-10-25T15:35:02Z**

Refined scope: AGENT_THINKING_LEVEL is the baseline (default medium); BYOK owners can persist a thinking override alongside their key or choose Use server default. Hosted/platform sessions ignore personal and saved conversation levels and use only the environment baseline. Added migration 037, API validation and omission-preserving updates, a dropdown, small-screen scrolling, documentation, and desktop/phone screenshots. Passed go test ./internal/agent ./internal/api ./internal/store ./cmd/server -timeout=10m and focused Node tests. Browser checks verified save/reload/clear without key re-entry and inspected real Pi argv for personal-over-environment and hosted environment-only precedence, including an attempted request-body override. Existing-key upgrade and restart tests preserve ciphertext and preference inheritance.
