---
id: Exh-vdns
status: closed
deps: [Exh-qhfd]
links: []
created: 2026-10-23T05:19:53Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, vendor]
---
# [Experimental] Swappable vendor credential engines: Composio alongside Nango, chosen per provider

EXPERIMENTAL; part of Exh-g2vv; follows Exh-qhfd (Nango engine, built on feature/Exh-g2vv/relay-connectors).

## Why

Nango bills about $0.29 per stored connection per month (prorated, whether used or not; $50/month base), which a $10/month hosted plan can't absorb for power users. Composio (checked 2026-10-22, composio.dev/pricing) bills by usage instead:
- Hobby $0 (100K tool calls a month; pauses at the cap). Pro $29/month with $29 of credit; $0.0003 per tool call over.
- **Connected accounts are unlimited and free** on Hobby and Pro. Exception: Composio-managed OAuth apps cost $0.10 once per connection beyond 1K on paid plans.
- **Proxy execute** calls endpoints Composio hasn't wrapped, with Composio handling auth. Billed as a tool call plus +$0.0002 (1K a month free on Hobby): about $0.0005 per relayed request.
- Managed OAuth apps skip provider registration (Composio says they're 'not recommended for scaling'; 20K free calls, then $0.0005 per call). Bring-your-own apps are supported.
- No self-hosting (Enterprise offers only a customer-run keyring proxy), so requests and response data pass through Composio. Disclose this; it fits the hosted tier, not the self-hosted promise.

So hosted Exhibit wants Composio (or both) for the long tail; self-hosters want native plus optional self-hosted Nango (free edition: Auth + Proxy; Elastic License 2.0 forbids offering it as a hosted service).

## What to build

- **Generalize the Nango-specific vendor layer** (internal/connectors/vendor.go on the branch) into a vendor engine interface beside the existing `Engine` (connectors.go): connect flow (start, complete), authorize or send, status (ok, needs_reconnect, revoked), revoke. Nango and Composio are implementations; native stays as it is.
- **Choose the engine per provider, in operator config.** GitHub native, a long-tail service via Composio, another via Nango, all at once. connections record engine plus external_ref.
- **Policy stays in Exhibit, whichever engine answers:** grants, the allowlist, data-destination policy, quotas, caching, logs, stand-ins. A vendor never decides what is allowed.
- **Two delivery modes; prefer inject.**
  - *Inject:* the engine yields the credential and Exhibit's own egress gate makes the request. This is what Exh-qhfd chose for Nango (retrieve managed credentials, keep the single gate), because every redirect is then checked and reflected credentials can be scrubbed. Use it wherever the vendor lets us retrieve the credential.
  - *Forward:* used only if a vendor offers proxy only (verify for Composio). The vendor dials, so the relay must validate the target URL against the recipe and allowlist before handoff, and cap size and time on the response. It cannot enforce dial-guard or redirect checks on the vendor's hop, and cannot scrub reflected secrets it never held. Mark forward-mode providers as such in the grant UI and docs.
- **Normalize vendor errors** into one set (needs_reconnect, rate_limited, refused, upstream_error) so the relay, agent tools and UI behave the same across engines.
- **Cost accounting per engine and provider:** count stored connections (Nango) and calls (Composio), so an operator sees what each provider costs and can move a provider to native when cheaper. Relay caching, per-connection rate limits and the no-relay-for-widgets rule are now cost controls too.
- **Switching engines needs a migration story.** Tokens usually can't leave a vendor, so changing a provider's engine means new connections use the new engine and existing ones keep working until the user reconnects (prompted via needs_reconnect). Revocation is per engine.

## To verify before building Composio
- Whether proxy execute reaches arbitrary endpoints or only a connected toolkit's own API domain.
- Which methods and headers it forwards, and how errors and status codes come back.
- Whether connected-account credentials can be retrieved for inject mode.
- Data retention and logging terms for proxied request and response bodies.

## Acceptance Criteria

- One long-tail provider works through Composio and another through Nango on the same instance. The artifact and agent code don't change, and switching a provider's engine in config affects only new connections.
- Grants, allowlist, quota, caching and destination policy are enforced identically for native, Nango and Composio (shared test table).
- Forward-mode requests are refused before handoff when the target URL is outside the recipe; response caps apply.
- Vendor errors surface as the normalized set; needs_reconnect drives the reconnect prompt.
- Per-engine cost counters are visible to the operator.


## Notes

**2026-10-23T06:18:13Z**

Implemented a shared VendorEngine interface with Nango inject and Composio forward adapters selected per reviewed provider. Connections and pending flows pin the original engine, external reference and policy; migration 041 pins legacy Nango records without credential transfer. The common relay retains grants, destination custody, allowlists, quota/cache, limits and logs. Normalized errors persist reconnect state and drive trusted UI prompts. Monthly anonymous per-engine/provider management, inject and forward attempts plus stored connections are visible to operators. Updated configuration, API, architecture/security docs and desktop/mobile screenshots.

Verified current official Composio Connect Link, account, proxy, token-custody, revocation and retention contracts before implementation. Credential export is unsupported, requiring disclosed forward mode; provider dial/redirect validation and unknown-token scrubbing remain vendor limitations. Binary-download URLs are refused, and encoded uploads must fit the control-plane cap. Fixture coverage includes mixed engines, config switches and pending flows, common policy enforcement, lifecycle, errors, response caps and cost counters. Live consent, billing and actual project retention settings remain in Exh-3kvx; this environment has no vendor credentials.

Validation: make assets; full go test -json -timeout 15m ./... (all packages pass); go vet ./...; race suites for egress, connectors and bundle; final Composio MIME/string fixtures also pass with race detection. Browser checks covered generic Basic creation, encrypted metadata, limits and htmx updates, removal, desktop/mobile/dark views and refreshed gallery examples. The removed-provider tracked-file search is empty. A full store race probe exceeded its default ten-minute timeout in the large account deletion fixture; targeted affected store races and the full normal store suite pass.
