---
id: Exh-rrjy
status: open
deps: []
links: []
created: 2026-11-02T15:59:04Z
type: feature
priority: 1
assignee: Max Omdal
parent: Exh-qkl7
tags: [connectors, security, experimental]
---
# Bind connector ciphertexts to their recipe and keep the key off the data volume

Connector credentials are sealed by internal/secrets Box.Encrypt with no additional authenticated data, and the connection's recipe (origins, credential header, scheme) is stored as plaintext JSON beside the ciphertext (internal/connectors/connectors.go save/readCredential). Anyone who can write the database but does not hold the key — a tampered backup, write access to the Litestream bucket, a SQL bug — can change the stored origin, and the server will decrypt the key and inject it there. Encryption at rest is defeated without ever touching the key.

The key itself is also weak in placement: with no EXHIBIT_SECRET, secret.key is generated inside DATA_DIR (cmd/server/main.go), so any filesystem backup of the data volume carries both the ciphertexts and the key. Connector credentials share one AES key with BYO agent keys, and EXHIBIT_SECRET is only SHA-256 hashed, with no entropy check.

## Design

- Seal each connector credential with AAD = owner id, connection id and a hash of the canonical recipe (origins, header, scheme, private base). Decrypt fails if any of them changed. Reconnect re-seals under the same binding.
- Use a dedicated connector key derived through Box.DeriveKey rather than the shared AES key. Agent keys may move to their own purpose key in the same change.
- Prefix ciphertexts with a key/format version so rotation, and later a KMS backend (sibling ticket), can coexist with existing rows. A startup backfill re-seals legacy rows; unreadable rows report needs_reconnect rather than failing startup.
- When CONNECTORS_ENABLED, refuse to start if the only key is a secret.key inside DATA_DIR. Accept EXHIBIT_SECRET or a key file path outside the data volume. Reject an EXHIBIT_SECRET too short to carry 128 bits.
- Self-hosted instances need no new infrastructure.

## Acceptance Criteria

- A test edits a stored connection's recipe origin directly in SQLite and the next relay request fails to decrypt instead of sending the credential to the new origin. Same for copying a ciphertext onto another connection or owner.
- Legacy unversioned rows are re-sealed by the backfill and keep working.
- Startup with connectors enabled and only an in-DATA_DIR key fails with a clear message; startup with connectors disabled is unchanged.
- docs/relay-connectors.md and docs/security.md describe the binding and key placement.

