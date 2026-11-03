---
id: Exh-68sx
status: open
deps: [Exh-rrjy]
links: []
created: 2026-11-02T15:59:15Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-qkl7
tags: [connectors, security, hosted]
---
# AWS KMS envelope encryption for connector credentials on the hosted platform

On the hosted platform, wrap connector credentials (and BYO agent keys) with AWS KMS using envelope encryption, so a copy of the database and data volume is not enough to decrypt them. Self-hosted instances are unaffected: the KMS is selected by configuration, unset means the local key from Exh-rrjy, in the same shape as BLOB_S3_BUCKET.

The hosted platform runs on Fly.io. Fly's OIDC federation with AWS gives the machine short-lived AWS credentials from a role ARN in fly.toml, with no long-lived secret on the box (https://fly.io/docs/security/openid-connect/).

## Design

- One symmetric customer-managed KMS key for the platform (about $1/month), in the AWS region nearest the Fly region. Never one KMS key per user.
- Per connection: GenerateDataKey, encrypt locally, store the wrapped data key with the ciphertext under the version prefix from Exh-rrjy. Pass owner id and connection id as KMS encryption context, in addition to the local AAD, so CloudTrail attributes each decrypt to an owner.
- Cache unwrapped data keys in memory for a few minutes to bound cost, latency and short outages. Past the cache, fail closed with a retryable connection error; never fall back to a local key.
- A configured KMS that fails a test call at startup stops the server with a clear error, after a short retry for Fly's token file appearing late.
- Prefer Tink's Go KMS envelope AEAD (tink-go + tink-go-awskms) over a hand-rolled format if it supports the AAD/context binding; otherwise AWS SDK v2 kms + sts, isolated in its own package under internal/secrets and only loaded when configured.
- Render-token keys keep their local derivation (hot path).
- A backfill re-wraps existing local-key rows when the KMS is first enabled.

## Acceptance Criteria

- With the KMS configured, a stored connection's ciphertext cannot be decrypted with the database and data volume alone.
- Tests run against a fake KMS: wrapping, context mismatch refusal, cache expiry, outage fails closed, startup check.
- Unset configuration leaves self-hosted behavior and tests unchanged.
- Configuration documented in docs/deployment.md and docs/relay-connectors.md; the AWS/Fly hardening runbook is a separate ticket.

