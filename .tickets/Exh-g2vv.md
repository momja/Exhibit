---
id: Exh-g2vv
status: in_progress
deps: []
links: [Exh-co2m, Exh-h5va]
created: 2026-10-22T06:50:04Z
type: epic
priority: 2
assignee: Max Omdal
tags: [experimental, connectors, security]
---
# [Experimental] Relay and connectors: external APIs and user accounts for artifacts

EXPERIMENTAL. Exploratory direction, not committed roadmap. Validate each phase before building the next, and expect the design to change.

Goal: let artifacts use external APIs (including ones with no CORS support) and, later, the user's own accounts ("connectors"), without running artifact code on the server and without leaking credentials, keeping "it's just a file" intact.

## How we got here (2026-10-21/22 design discussion)

1. **A sandboxed server-side backend was prototyped first** (Exh-co2m, branch feature/Exh-co2m/backend-poc, report docs/backend-poc.md on that branch). Workers-style `fetch(request, env)` handlers in quickjs-ng on wazero, an egress gate, a frame-to-host relay, and a runnable bundle. It worked: CORS-free requests, one network rule for page and backend, containment tests, a standalone bundle under Node and exhibit-run.
2. **But it is slow and heavy.** 17-400x slower than the browser's JIT (a 1M-iteration loop: 1.3 s vs 4.5 ms; a 100-result arXiv search: about 1.8 s vs 300 ms on Node). Deadline enforcement alone costs 4-6x. Shipping the whole localStorage snapshot into each instance added about 200 ms per request at 513 KB of state. It also adds a code-execution runtime to the attack surface.
3. **Most personal apps don't need a backend; they need a relay.** A CORS proxy behind the same egress gate keeps logic in the page (fast, ordinary code, nothing executing on the server) and covers most cases. What a relay can't do: work while no page is open (schedules), inbound webhooks, shared state with enforced rules, server-side reduction, hiding raw data from share recipients. Only scheduled work is common for personal apps, and it may be covered later by declarative scheduled fetches, or by running the artifact itself in the existing headless Chromium sidecar. The backend is parked until one of those needs is concrete. (A separate serverless-backend effort, Exh-h5va, is in progress; reconcile with it.)

## Decisions

- **One relay for everything.** Unauthenticated and connector requests take the same path through the same gate (internal/egress from the POC). Never two proxies.
- **Treat frame messages as hostile.** The browser's forbidden-header rules don't apply to a postMessage payload, so the relay enforces a header allowlist itself. The POC gate's header denylist would let a crafted Cookie header through; fix that first.
- **Relay use is its own consent, not a side effect of `allow`.** Copied sites (URL ingest) call their own backends constantly: form posts, autosave, analytics. Through a relay those requests would start working, leave from the server's IP, and turn snapshots into live clients. Existing allow rows must not silently gain relay power. Writes need a stronger grant than reads.
- **Agents aren't tricked; they're told a platform fact.** Like the storage fact already at internal/agent/prompt.go:22, which currently says "prefer zero external references". Models will otherwise reach for public CORS proxies (corsproxy.io, allorigins), backends, or `mode: "no-cors"`.
- **Connector recipes are security policy, not integration code.** Agents must not write them: an injected agent could route a secret into an echoed field or to an attacker's origin. Recipes are declarative data: built-in, generic (API key / Bearer / Basic), or agent-drafted but human-approved, with the approval text generated mechanically from the recipe. Recipes that must sign requests (AWS SigV4, HMAC) are first-party code only.
- **Credentials are acquired only in Exhibit's own UI.** An API key, password, one-time setup or claim token, or OAuth consent never passes through artifact code. A one-time token is itself a credential: pasted into an artifact, it could be sent elsewhere and redeemed by an attacker.
- **Start with attach-by-origin.** The artifact calls the API with no auth header; the relay adds the credential when this artifact holds a grant for that origin's connection. Add stand-in tokens later, for recipes whose ordinary clients expect to hold a credential value (the Meta Muse pattern: stand-ins in the runtime, real secrets substituted at egress).
- **Connections belong to the viewer by default.** Sharing your own connection with a recipient is an explicit opt-in. Without that rule, shared-state artifacts would inline the owner's credential into a recipient's frame.
- **Grants state where data can go.** Stand-ins and injection protect the credential, not the data. A connector grant must show "can read X; can send data to: ...", and financial recipes default to "nowhere else".
- **Economics.** Nango Pay-as-you-go is $0.29 per stored connection per month (prorated, charged whether used or not) plus a $50/month base; the free plan caps at 10. Its free self-hosted edition is Auth + Proxy only, and its Elastic License 2.0 likely forbids bundling it into a hosted Exhibit without a commercial agreement. A $10/month plan can't absorb that per connection. So: native secrets (API key / Bearer / Basic) cost about $0; native OAuth for the top few providers is a fixed per-provider cost (Google's restricted scopes need a yearly CASA assessment); a vendor serves only the long tail, under a small entitlement cap, an add-on pack, and idle-connection pruning.
- **No provider-specific connectors beyond the reviewed OAuth set.** Services that use API keys, Bearer tokens or Basic auth (including access URLs with embedded credentials) go through the generic connectors. Even when an API sends permissive CORS headers, the hard parts are credential custody (a long-lived bearer secret) and quota (some APIs disable tokens that exceed a daily budget), so generic connectors carry per-connection rate and cache limits and default to "data may go nowhere else". fetch() refuses URLs with embedded credentials, so the client must send a Basic header.
- **Host threats the relay must address:** SSRF (dial-time guard, every redirect hop, infrastructure-level egress isolation); open relay and abuse from our IP (auth only, per-owner/artifact/target limits, 429 back-off, honest User-Agent); resource exhaustion (decoded-size caps, timeouts); data liability (metadata-only logs, never bodies or query strings); confused deputy (the bridge pins the artifact id).

## Phases (child tickets)

1. Relay without credentials, plus the agent prompt change and footprint warnings.
2. Generic native connectors (API key / Bearer / Basic, including pasted URLs with embedded credentials), the connect UI, grants, the agent tools, per-connection rate and cache limits.
3. Native OAuth for the first providers people ask for, plus entitlement caps by connection kind.
4. A vendor credential engine (Nango) for the long tail, plus idle pruning.
5. Stand-in tokens.
6. Declarative scheduled fetches, so widgets and data stay fresh without an open page.
7. Standalone parity: local relay and connections in serve.mjs / exhibit-run.

## Acceptance Criteria

- Each phase ships behind an operator flag and stays labeled experimental until reviewed.
- architecture.md, security.md, api.md and the PRD are updated as phases land. "Execution stays in browsers" remains true.
- A decision is recorded with Exh-h5va about the parked backend: reconcile, supersede, or keep for scheduled, webhook and shared-authority needs.

