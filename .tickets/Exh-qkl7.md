---
id: Exh-qkl7
status: open
deps: []
links: []
created: 2026-11-02T15:58:52Z
type: epic
priority: 1
assignee: Max Omdal
tags: [connectors, security, experimental]
---
# Harden connector credential custody

Connectors let users store personal API keys. Artifact code already never sees a key: the relay injects it per request at the connection's exact origin and scrubs reflected copies from responses. The remaining paths to a stolen or abused key are around that boundary:

- A connection grant outlives the code it was granted to.
- A granted tool can use the key to mint a new credential or plant persistence, which scrubbing cannot catch.
- At-rest encryption can be bypassed by anyone who can write the database: there is no AAD, and the recipe (origin, header) is plaintext beside the ciphertext.
- The local key lives next to the database in DATA_DIR.
- The app origin has no script-src CSP on the pages where keys are typed.
- Users can be talked into pairing a key with a look-alike origin.

The hosted platform (Fly.io) additionally wraps credentials with AWS KMS (envelope encryption). Self-hosted instances keep a local key and never require a KMS.

