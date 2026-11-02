---
id: Exh-61g2
status: open
deps: []
links: []
created: 2026-11-02T15:59:41Z
type: feature
priority: 1
assignee: Max Omdal
parent: Exh-qkl7
tags: [connectors, security, experimental]
---
# Refuse credential-minting endpoints and redact credential-shaped responses

Response scrubbing (responseScrubber in internal/connectors/standins.go) removes only the stored credential and a few encodings of it. A granted tool does not need the original key if it can use it to create a new credential or plant persistence: add an attacker's SSH key with POST /user/keys on GitHub, point an org webhook elsewhere, add a deploy key or collaborator, or, with an admin-scoped key, mint a fresh API key that comes back in the response body. NativeEngine.Authorize checks only the origin.

This is not the read/write split removed by Exh-cxzb. These are endpoints a library tool never legitimately needs, refused regardless of method semantics.

## Design

- A reviewed per-provider deny list of credential and persistence endpoints (method + path pattern), applied before credential injection for native, OAuth and inject-mode vendor connections, and checked on every redirect hop. Start with GitHub (keys, gpg_keys, deploy keys, hooks, collaborators, authorizations/apps, tokens) and the common API-key providers' key-management routes. Custom connections get the list for their origin's known provider, if any.
- Redact credential-shaped strings in connector responses, not only the stored value: known token formats (ghp_, gho_, ghs_, github_pat_, sk-, sk-ant-, xoxb-/xoxp-, AKIA/ASIA ids, and similar), plus base64, base64url and hex forms of the stored bearer value.
- Refusals return a stable error code the host chrome can explain.
- Forward-mode (Composio) applies the deny list to the sanitized request before forwarding.

## Acceptance Criteria

- Tests per provider pattern, including a redirect into a denied path and a case-variant path.
- A fixture response containing a newly minted ghp_ / sk- token is redacted before reaching the frame, cache or saved state.
- The documented deny list lives in one reviewed place with its rationale; docs/relay-connectors.md explains that key-management tools are out of scope.

