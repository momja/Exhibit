---
id: Exh-7y0c
status: closed
deps: []
links: []
created: 2026-11-16T06:04:05Z
type: chore
priority: 2
assignee: Max Omdal
tags: [deployment, mcp, hosting]
---
# Serve the MCP endpoint on a stable public hostname instead of the Fly machine domain

The hosted instance advertises the optional MCP service on the Fly machine's
default `<app>.fly.dev` hostname. That address is not a product surface: it is
machine-specific, it is not a domain we control or can brand, it changes if the
app is renamed, and it is the address users copy into their agent config
(`claude mcp add --transport http exhibit <MCP_URL>`) — a permanent, personal
config entry pointing at an infrastructure detail.

## Current state

- `MCP_PUBLIC_URL` (the `exhibit-mcp` service) and `MCP_URL` (the core) must
  name the same public endpoint; see `docs/deployment.md` §5.3.
- `MCP_PUBLIC_URL`'s origin is the only accepted browser `Origin` for MCP
  requests, and it is what settings, the copyable command and OAuth discovery
  advertise. Changing it changes all of those at once.
- The core never serves `/mcp` (Exh-uhvu). `exhibit-mcp` is a separate process
  (`MCP_ADDR`, default `:8090`) with no database, operator token or secrets of
  its own; every request is checked against the core.
- On Fly, `SINGLE_LISTENER` dispatches by `Host` between the app origin and the
  renderer only (`fly.toml`). There is **no** MCP host branch, so a new hostname
  does not reach `exhibit-mcp` without one of the two shapes below.

## Decision this chore has to record

1. **Path on the existing app host** — `https://<apex>/mcp`, with the proxy
   routing that one path to the `exhibit-mcp` process. Matches the documented
   Compose example, keeps one certificate and one hostname, and tucks MCP under
   the product origin (so `EXHIBIT_PUBLIC_URL == APP_ORIGIN` stays true).
2. **Its own hostname** — e.g. `https://mcp.<apex>/mcp`, needing a Fly cert plus
   DNS, and either a second app/process for `exhibit-mcp` or a `SINGLE_LISTENER`
   extension that dispatches that host to it. Cleaner separation, more moving
   parts, and `EXHIBIT_PUBLIC_URL` must still remain the core's `APP_ORIGIN`.

Pick one, write down why, and pin it in the deployment config so it cannot drift.

## Acceptance criteria

1. A chosen public hostname is documented (one of the two shapes above, with the
   reason) and used for both `MCP_URL` and `MCP_PUBLIC_URL`.
2. TLS terminates correctly on the chosen hostname and plain HTTP redirects.
3. No `*.fly.dev` hostname appears in: Settings → Connected agents, the
   copyable `claude mcp add` command, `/.well-known/oauth-protected-resource`,
   or `/.well-known/oauth-authorization-server` metadata.
4. `EXHIBIT_PUBLIC_URL` still equals the core's `APP_ORIGIN`, so the OAuth
   issuer does not move and existing signed client registrations stay valid.
5. End-to-end connect works from a clean machine: `claude mcp add --transport
   http exhibit <MCP_URL>`, browser approval, one tool call against the library.
6. Existing grants still list and revoke in Settings; only clients configured
   with the old address need re-adding. Say so in the docs.
7. `docs/deployment.md` §5.1/§5.3 state how the chosen shape is wired on Fly
   (cert, DNS, process or dispatch), not only on Compose.


## Notes

**2026-11-16T07:10:19Z**

Decision: use https://exhibit.dizzard.net/mcp for both MCP_URL and MCP_PUBLIC_URL; keep EXHIBIT_PUBLIC_URL equal to APP_ORIGIN (https://exhibit.dizzard.net). Existing app-host path reuses product DNS/certificate and preserves OAuth issuer/client registrations, avoiding another hostname/cert. Repository implementation only; no deployment, DNS or certificate mutation authorized. Live TLS/redirect and clean-client OAuth/tool-call verification remain pending.

**2026-11-16T20:58:37Z**

Repository design confirmed: reuse https://exhibit.dizzard.net/mcp for MCP_URL and MCP_PUBLIC_URL while EXHIBIT_PUBLIC_URL remains APP_ORIGIN. Exact app-host MCP/discovery paths forward to a loopback-only separate satellite; render host wins and browser cookies are stripped. Fly app=fly supplies the image ENTRYPOINT argument, supervising both processes on the same volume-backed machine; a separate process group would create another machine and break loopback routing. No deployment or cloud/DNS mutation performed. Closing is repository implementation only: live DNS/certificate/HTTP redirect checks, old-client re-add, existing hosted grant listing/revocation and clean-machine Claude OAuth/tool-call acceptance remain pending an explicitly authorized operator rollout. Independent final-head reviews and merge gates are owned by the parent; implementer must not merge.
