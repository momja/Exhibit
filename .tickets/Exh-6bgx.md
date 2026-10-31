---
id: Exh-6bgx
status: open
deps: []
links: [Exh-17a5]
created: 2026-10-31T16:24:06Z
type: chore
priority: 2
assignee: Max Omdal
tags: [deploy, mcp, fly, hosted]
---
# Enable MCP on the hosted Fly deployment

Turn on MCP access for bring-your-own agents (epic Exh-17a5) on the hosted Fly deployment. The deploy-side work is in momja/exhibit-deploy PR #5 (https://github.com/momja/exhibit-deploy/pull/5); its README "MCP rollout" section has copyable verification commands and optional custom-domain DNS/certificate setup.

Needs one new Fly app, one deploy token, four GitHub environment variables, and one core Fly secret. No volume, no copied core secrets, no Clerk changes, no MCP runtime secrets.

## Steps

1. **Release prerequisites.** Merge the Exhibit MCP stack (#234-#239) and exhibit-deploy PR #5. Update `exhibit-revision` to a merged Exhibit commit containing MCP and the built-in OAuth server, and deploy that revision with MCP still disabled.
2. **Create the MCP app** in the core's Fly organization:
   ```bash
   fly apps create YOUR_MCP_APP --org YOUR_ORG
   fly ips allocate-v4 --shared --app YOUR_MCP_APP
   fly ips allocate-v6 --app YOUR_MCP_APP
   ```
   Do not create a volume or copy core secrets. Endpoint: `https://YOUR_MCP_APP.fly.dev/mcp` (no custom DNS or certificate needed).
3. **Deploy token** for it, stored beside the existing `FLY_API_TOKEN` (keep that one):
   ```bash
   fly tokens create deploy --app YOUR_MCP_APP \
     --name github-exhibit-mcp-deploy --expiry 8760h
   gh secret set FLY_MCP_API_TOKEN --repo momja/exhibit-deploy --env FLY_APP
   ```
   Paste the full token, including the `FlyV1 ` prefix. Expires in one year.
4. **GitHub → momja/exhibit-deploy → Settings → Environments → `FLY_APP` variables:**
   - `FLY_MCP_APP`: the new app name.
   - `EXHIBIT_PUBLIC_URL`: the core's exact HTTPS `APP_ORIGIN`, no trailing slash (gallery hostname, not the render origin).
   - `MCP_PUBLIC_URL`: `https://YOUR_MCP_APP.fly.dev/mcp`.
   - `FLY_MCP_ENABLED`: `true`.
5. **Run "Deploy production" on `main`**, then enable MCP on the core (restarts it):
   ```bash
   fly secrets set --app YOUR_CORE_APP MCP_URL=https://YOUR_MCP_APP.fly.dev/mcp
   ```
   `MCP_URL` must exactly match `MCP_PUBLIC_URL`.
6. **Verify:** MCP `/healthz` succeeds; unauthenticated `POST /mcp` returns 401 with `WWW-Authenticate: Bearer resource_metadata=...`; the metadata names the gallery origin as authorization server. Then connect Claude Code from Settings → Connected agents (`claude mcp add --transport http exhibit <MCP_URL>`, `/mcp` → Authenticate, approve).

## Acceptance Criteria

Claude Code connects to the hosted instance through browser sign-in, searches and edits an artifact, the connection is listed in Settings → Connected agents with its client name, and revoking it makes the next MCP request fail. The MCP app has no volume and no core secrets; FLY_MCP_API_TOKEN's expiry is noted for renewal.

