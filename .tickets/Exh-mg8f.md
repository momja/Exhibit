---
id: Exh-mg8f
status: open
deps: []
links: []
created: 2026-10-22T06:50:51Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, security, relay]
---
# [Experimental] Phase 1: relay for cross-origin requests (no credentials)

EXPERIMENTAL; part of Exh-g2vv. The CORS-free fetch path every later phase builds on.

- **Shim routing:** cross-origin fetch() to an allowlisted origin, from an artifact holding the relay grant, goes over the host bridge (the POC's frame-to-host relay, with the artifact id pinned by the page) to POST /api/artifacts/:id/relay. Framed and logged-in viewers only; never public-link visitors or the top-level render. fetch first, XMLHttpRequest later.
- **Relay grant:** a per-artifact capability with a first-use prompt in app chrome, separate from allow rows, so copied sites and existing approvals gain nothing silently. Writes (POST/PUT/PATCH/DELETE) need a stronger grant than GET/HEAD.
- **Gate hardening, on top of the POC's internal/egress:** a header allowlist (Accept, Content-Type, Accept-Language, If-None-Match, Range, ...) replacing the denylist; never forward Cookie, Authorization, Origin, Referer or X-Forwarded-*; strip Set-Cookie and hop-by-hop headers; send User-Agent 'Exhibit-Relay/1 (+instance URL)'; https only. Already in the POC: allowlist checked on every redirect hop, the dial-time private-address guard, undialable destinations refused unapprovably, decoded-size and time caps, no environment proxy.
- **Limits:** token buckets per owner, per artifact and per target origin; instance-wide back-off on 429 and Retry-After; response caching where allowed.
- **Operations:** metadata-only logs (owner, artifact, origin, method, status, bytes, latency; never bodies or query strings); a relay activity panel beside the allowlist; RELAY_ENABLED kill switch, off by default on public instances; operator allow and deny lists of target origins.
- **Copied sites:** URL ingest reports when a snapshot still calls its original site's API ('this copy still calls its original site's server').
- **Response delivery:** bytes go to the opaque frame as a Response only, never parsed or inserted on the app origin.

## Acceptance Criteria

- An SSRF suite passes: decimal and octal IPs, IPv4-mapped IPv6, DNS rebinding, redirects to private addresses, public names resolving inward.
- A crafted Cookie, Host or Origin header in a frame message never leaves the relay.
- Relay bytes never reach the app origin's DOM (test).
- An existing artifact with allow rows but no relay grant gets no relay behavior.
- The arXiv demo works in the page with no server script.

