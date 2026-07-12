# Exhibit — Plugin Ecosystem (design)

Status: **design** — no implementation yet. Ticket tree under epic `Exh-i0ll`
(see Tickets section). Companion to `docs/agent.md`: the agent surface is the
prototype this design is extracted from, and becomes the reference plugin.

## 1. Goal

Let third-party developers extend a base exhibit install — new ingest sources,
automations, alternate views, AI surfaces — **without forking the binary and
without weakening the deploy promise** (one small static Go binary, one SQLite
file, browser-enforced security boundaries).

Non-goals, decided up front:

- **No in-process plugins.** Go has no viable dynamic loading (`plugin` is
  platform-limited and toolchain-version-locked). Server-side plugin *code*
  inside the exhibit process means forks, not plugins. Never.
- **No pi-extension loading.** The agent sidecar could in principle load
  third-party Pi extensions as agent tools; deliberately out of scope — the
  agent runs with exactly the one embedded exhibit extension.
- **No plugin marketplace/distribution story yet.** Install is
  operator-explicit (a URL + token grant). Discovery/registry can come later
  without changing the contract.

## 2. The shape: plugins are API clients, UIs are sandboxed iframes

Exhibit already has the two properties a plugin system needs:

1. **The single write path.** Every mutation goes through the HTTP API, so
   anything holding a token is already a well-behaved extension point. The
   thumbnail worker, the future Chrome extension, and the agent surface are
   all this shape today.
2. **Origin-isolated untrusted UI.** Rendering untrusted HTML in sandboxed
   iframes with postMessage bridges is exhibit's core competency. Plugin UIs
   get the same treatment as artifacts: the boundary is the browser's, not a
   code-review promise.

That yields two plugin tiers on one contract:

- **Headless plugins** — a process (anywhere) holding a scoped token, calling
  the API, optionally subscribed to the event feed. No exhibit changes needed
  beyond scopes + events.
- **UI plugins** — headless plugins that additionally declare mount points in
  a manifest. Exhibit renders their contributions (nav link, artifact-toolbar
  action, full page) inside sandboxed iframes served from the *plugin's own
  origin*, bridged to exhibit through origin-pinned postMessage — the same
  mechanics as the storage shim and snippet picker.

## 3. Trust model: three tiers

| Tier | Who | Boundary |
|------|-----|----------|
| Artifacts | Untrusted (any file) | iframe sandbox + per-artifact CSP (unchanged) |
| Plugins | Operator-installed, least-privilege | scoped token server-side; sandboxed iframe + scoped postMessage bridge client-side |
| Core | The exhibit binary | full datastore access |

Plugins are trusted the way installed software is trusted — the operator chose
them — but are still least-privilege: a broken or malicious plugin is bounded
by its token scopes and its iframe. Plugins never see BYO provider keys or
other plugins' tokens; the key-handling discipline from the agent design
carries over unchanged.

## 4. The four seams to build

### 4.1 Scoped API tokens (`Exh-mety`)

Today the single static token grants everything, so any plugin is root. The
plugin contract needs per-plugin tokens carrying capability scopes:

```
tokens(id, owner_id, plugin_id, name, hash, scopes, created_at, revoked_at)
scopes: artifacts:read | artifacts:write | state:read | state:write
        | tags:write | collections:write | shares:write | events:subscribe
```

- Attaches to the existing auth-middleware seam (the same seam av-q30x uses
  for real sessions); handlers gain a required-scope annotation per route.
- The operator's own token keeps full scope; plugin tokens are minted and
  revoked in a settings UI.
- Token hash at rest (like the agent keys: never plaintext in the DB).

### 4.2 Event feed (`Exh-bujj`)

Reactive plugins need push, not polling. One SSE firehose (reusing the SSE
plumbing built for agent sessions) plus optional webhooks:

```
GET /api/events            SSE, scope events:subscribe
  artifact.created / artifact.updated / artifact.deleted
  state.changed(artifact_id, key)
  share.created / share.deleted
webhooks(id, plugin_id, url, event_filter, secret)   -- optional, phase 2
```

Events carry ids, not bodies (the plugin fetches what its scopes allow).
Delivery is at-most-once for SSE (reconnect = catch up via list endpoints);
webhooks get signed payloads (HMAC with the webhook secret).

### 4.3 Manifest, registry, UI mount points (`Exh-dicw`)

```jsonc
// served by the plugin at <base_url>/exhibit-plugin.json
{
  "name": "agent",
  "version": "0.1.0",
  "base_url": "https://agent.example.com",
  "scopes": ["artifacts:read", "artifacts:write", "events:subscribe"],
  "mounts": {
    "nav":              { "label": "Agent", "icon": "robot", "path": "/ui" },
    "artifact_toolbar": { "label": "Modify with agent", "icon": "robot",
                          "path": "/ui?artifact={artifact_id}" }
  }
}
```

- **Install flow:** operator pastes the manifest URL → exhibit fetches and
  displays name/scopes/mounts → operator approves → a token with exactly
  those scopes is minted and delivered to the plugin (one-time exchange).
  Mirrors the artifact ingest philosophy: surface the footprint, explicit
  approval, nothing granted by default.
- **Mount rendering:** `nav` adds a gallery-header link to an exhibit-served
  wrapper page embedding `<iframe src="<base_url><path>" sandbox="allow-scripts allow-same-origin">`.
  `allow-same-origin` is safe *here* (unlike artifacts) because the plugin
  runs on its own third origin — it gets its own storage/cookies but still
  can't touch the app or render origins. `artifact_toolbar` adds the action
  to the detail page with `{artifact_id}` substituted.
- **Host bridge:** the wrapper page exposes a small origin-pinned postMessage
  API (the storage-shim pattern): `getContext()` → current artifact id/title,
  `requestFetch(path, opts)` → proxied API call executed by the host with the
  plugin's token and checked against its scopes. Plugin pages therefore need
  no CORS against exhibit and never hold the token in the iframe.
- Registry is a table (`plugins(id, name, base_url, manifest, enabled)`) plus
  enable/disable in settings — disable unmounts UI and revokes the token's
  validity without uninstalling.

### 4.4 API contract v1 (`Exh-7o9d`)

Plugins outlive refactors only if the API is a promise. Deliverable: a
versioned `docs/api.md` declaring which routes/fields are stable (v1), a
deprecation policy, and an `X-Exhibit-API-Version` response header. No new
functionality — documentation and discipline.

## 5. The agent as reference plugin (`Exh-k75k`)

The agent surface (epic `Exh-yvhp`, `docs/agent.md`) is already plugin-shaped:
HTTP-only library writes, its own UI page, SSE streaming. Extraction order:

1. **Transcripts through the API** (`Exh-v6v4`, independent, do first):
   `Session.persistTranscript` currently calls `store.SaveTranscript`
   directly — the one violation of the single write path. Becomes
   `PUT /api/artifacts/:id/transcripts` + a handler; the direct store call
   goes away. Worth doing even if extraction never happens.
2. Move `internal/agent` + handlers + chat page behind the manifest contract:
   agent becomes a plugin with `nav` + `artifact_toolbar` mounts and
   `artifacts:read/write, events:subscribe` scopes, whether it stays
   in-repo (compiled in, registered as a "built-in plugin") or moves to its
   own repository. The manifest/mount design is validated by making the
   richest surface we have fit it.

The snippet picker stays in core (`internal/render`) — it is a render-surface
capability any plugin page embedding an artifact can drive via postMessage,
not agent-specific code.

## 6. Sequencing

1. `Exh-v6v4` transcripts via API (small, standalone, pays for itself)
2. `Exh-mety` scoped tokens → headless plugins become real
3. `Exh-bujj` event feed → reactive plugins
4. `Exh-7o9d` API contract v1 (can run parallel to 2)
5. `Exh-dicw` manifest + mounts → UI plugins
6. `Exh-k75k` agent refit as the reference plugin

## 7. Open questions

- Whether plugin wrapper pages should get per-plugin subdomains in the
  hardened deployment (mirrors the per-artifact-subdomain option, §12 of the
  tech stack doc) or one shared "plugins origin" is enough for v1.
- Webhook delivery guarantees (retry with backoff vs fire-and-forget) —
  proposal: fire-and-forget in v1, SSE is the reliable path.
- Whether `state.changed` events should be throttled per artifact (a chatty
  artifact could flood the feed).
- Multi-user interaction (av-q30x): plugin installs are per-instance in v1;
  per-user plugin enablement is a later concern.
