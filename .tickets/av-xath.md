---
id: av-xath
status: in_progress
deps: []
links: [av-lvxq]
created: 2026-08-19T06:02:38Z
type: feature
priority: 2
assignee: Max Omdal
tags: [deployment, infra, security]
---
# Serve both origins from one listener, dispatched by Host

Exhibit binds two listeners — ADDR for the app surface, RENDER_ADDR for the render surface (cmd/server/main.go:203-209) — and relies on an operator proxy to map a hostname to each port. Platforms whose proxy routes by port rather than Host header (Fly.io) give one public 443 that lands on one internal port, so both hostnames arrive at the same handler and the two origins collapse into one. Since the origin split IS the artifact sandbox boundary (architecture.md 3.2, 4), collapsing it is not a routing inconvenience — it puts /api/* on the origin where artifact code runs.

Add an opt-in mode that serves both surfaces from a single listener, dispatching each request to the app or render handler by its Host header. Blocks av-lvxq (fly.toml).

## Design

1. OPT-IN, ABSENT MEANS ABSENT. SINGLE_LISTENER unset keeps today's two listeners byte-identical — the OIDC_ISSUER shape. Only a deployment that has one port opts in.

2. THE DISCRIMINATOR IS RENDER_ORIGIN'S HOST, parsed once at startup, compared case-insensitively with the port stripped. Everything else falls through to the app router, so reaching the machine by IP or by a platform-assigned hostname (appname.fly.dev) gets the authenticated app surface rather than the render surface.

3. FAIL CLOSED ON A COLLAPSED BOUNDARY. If SINGLE_LISTENER is set and APP_ORIGIN and RENDER_ORIGIN resolve to the same host, refuse to start. A process that served both surfaces on one origin would be silently insecure, and the failure must not be discoverable only by noticing an artifact can reach the API.

4. WHERE IT LIVES. Not in main — main is untestable. A constructor in internal/api taking the two handlers and the render host, so the dispatch table is exercised by a normal in-process test.

5. THE TEST IS THE POINT. Both directions asserted: a render-Host request must not reach any app route (/api/*, /auth/*, gallery pages), and an app-Host request must not reach /a/:id, /w/:id or /s/:id. Route-walking rather than a handful of examples, so a route added later is covered by construction.

## Acceptance Criteria

1. SINGLE_LISTENER unset: two listeners, unchanged behaviour.
2. SINGLE_LISTENER set: one listener on ADDR serves both surfaces, dispatched by Host.
3. A request whose Host is RENDER_ORIGIN's host reaches only render routes; every app route 404s.
4. A request whose Host is anything else reaches only app routes; /a/:id, /w/:id, /s/:id 404.
5. SINGLE_LISTENER with APP_ORIGIN and RENDER_ORIGIN on the same host refuses to start, naming the reason.
6. An artifact renders in its iframe with its per-artifact CSP header intact when served this way.

