# Exhibit — API Reference

Companion to `product_requirement_doc.md` (the *what* and the boundaries) and
`architecture.md` (the *how* — see §3.1, the API surface as the single write
path). This document is the concrete HTTP API: every route, its auth, and the
ingest, state, sharing, and render flows a client uses.

All routes require `Authorization: Bearer <token>` except public share links.

## Artifacts

```
POST   /api/artifacts              Ingest an artifact (inline body, or url to fetch once)
GET    /api/artifacts              List artifacts (?q=search&tags=a,b&collections=c)
GET    /api/artifacts/:id          Get artifact metadata (?body=true for source)
PATCH  /api/artifacts/:id          Update title, body, network_allowlist, etc.
                                   (network_allowlist is the whole approved set; it
                                   replaces the artifact's allow decisions and leaves
                                   any blocked origins untouched)
POST   /api/artifacts/:id/refetch  Re-fetch body from source_url (URL-ingested artifacts)
DELETE /api/artifacts/:id          Delete artifact and associated rows (blob body is orphaned in v1)
```

**Ingest flow** — two steps by design:

```bash
# Step 1: scan (no network_allowlist → returns footprint, saves anyway)
curl -X POST http://localhost:8080/api/artifacts \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{"title":"My Tool","body":"<html>...</html>"}'

# Response includes network_footprint (origins the artifact references)
# Approve them by patching the allowlist:
curl -X PATCH http://localhost:8080/api/artifacts/<id> \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{"network_allowlist":["https://cdn.jsdelivr.net"]}'
```

Or approve at ingest time:

```bash
curl -X POST http://localhost:8080/api/artifacts \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{"title":"My Tool","body":"<html>...</html>","network_allowlist":["https://cdn.jsdelivr.net"]}'
```

**Ingest from a URL** — send `url` instead of `body` and the server fetches the
page (bounded to 10 MiB). The title falls back to the page's `<title>`. Because
the fetched page's relative references (`js/app.js`, `/assets/x.png`,
`url(bg.png)`) would otherwise resolve against the render origin and 404, a URL
ingest always injects a `<base href="<source-url>">` so they resolve against the
source site instead.

```bash
curl -X POST http://localhost:8080/api/artifacts \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/tool.html"}'
```

**Snapshot (self-contained vendoring)** — add `"snapshot": true` to a URL ingest
and the server fetches the page's own assets (images, styles, scripts, fonts,
including nested CSS `@import`/`url()` chains) and inlines them into the stored
document as `data:` URIs and inline `<script>`/`<style>`. The artifact becomes a
genuinely self-contained file that renders identically even if the source site
later disappears, and a fully vendored page collapses its network footprint
toward `connect-src 'none'`. Fetching is bounded (per-asset and total size caps,
an asset-count cap, timeouts, and an SSRF guard against non-public addresses).

`snapshot` requires `url`; requesting it on a pasted `body` is a `400`. Partial
failure never aborts the ingest — assets that can't be inlined (404, over a
limit, runtime-constructed URLs) keep their original reference (still resolvable
via the injected `<base href>`) and are reported. The response carries a
`snapshot` report:

```jsonc
{
  "artifact": { "id": "…", "title": "…", … },
  "network_footprint": ["https://source.example.com"],  // residual origins to approve
  "snapshot": {
    "applied": true,
    "vendored_urls": ["https://source.example.com/app.js", …],
    "vendored_bytes": 151723,
    "residual_origins": ["https://source.example.com"], // couldn't be inlined
    "failures": [
      { "ref": "img/missing.png", "url": "https://source.example.com/img/missing.png",
        "kind": "http-status", "detail": "unexpected status 404 Not Found" }
    ]
  },
  "render_url": "https://artifacts.example.com/a/…"
}
```

As with any ingest, residual origins surface in `network_footprint` for
**explicit** approval — the snapshot never seeds the `network_allowlist`, so a
snapshotted artifact stays network-inert until you approve its residual origins.

## State (cross-device sync)

```
GET  /api/artifacts/:id/state      Get all state key-value pairs
PUT  /api/artifacts/:id/state      Set one key {"key":"...","value":"..."}
```

The storage shim intercepts `localStorage`/`sessionStorage` in the iframe. Reads are served from state **inlined into the shim at render time** (so `getItem` is correct synchronously); writes are **`postMessage`-ed to the host frame**, which performs the authenticated `PUT` above (the sandboxed iframe has an opaque origin and can't call the API itself). No artifact changes needed — any tool that uses standard storage APIs gets cross-device sync automatically.

## Collections & Tags

```
GET    /api/collections                              List collections
POST   /api/collections                              Create collection {"name":"..."}
POST   /api/artifacts/:id/collections/:collectionID  Add to collection
DELETE /api/artifacts/:id/collections/:collectionID  Remove from collection

GET    /api/tags                                     List tags
POST   /api/tags                                     Create tag {"name":"..."}
PATCH  /api/tags/:id                                 Rename or recolor a tag
DELETE /api/tags/:id                                 Delete tag (cascade)
POST   /api/artifacts/:id/tags/:tagID                Add tag
DELETE /api/artifacts/:id/tags/:tagID                Remove tag
```

## Agent (build/modify with AI, BYO key)

```
PUT    /api/agent/key                        Store provider API key {"provider","model","api_key"} (encrypted at rest)
GET    /api/agent/key                        Key status (masked hint only — the key is never returned)
DELETE /api/agent/key                        Remove the stored key
POST   /api/agent/sessions                   Start a session {"artifact_id"?: bind to an existing artifact}
POST   /api/agent/sessions/:id/prompt        Send a prompt {"message", "images"?: [{data, mime_type}]}
POST   /api/agent/sessions/:id/abort         Abort the current run
DELETE /api/agent/sessions/:id               End the session
GET    /api/agent/sessions/:id/events        SSE event stream (?token= auth — EventSource can't set headers)
GET    /api/artifacts/:id/transcripts        Agent conversations persisted with an artifact
```

Each session spawns a [Pi](https://github.com/badlogic/pi-mono) sidecar
(`pi --mode rpc`) whose only tools call back into this API, so agent output
enters the library through the same ingest path (scan + explicit allowlist
approval) as everything else. The chat UI lives at `/agent`
(`/agent?artifact=<id>` to modify an existing artifact); snippet mode
(Ctrl+Shift+S) lets you click an element in the live preview and attach its
screenshot + selector to your next prompt. See [docs/agent.md](./docs/agent.md).

## Shares

```
POST   /api/shares                 Create share {"artifact_id":"...","public":true}
DELETE /api/shares/:id             Delete share
GET    /s/:shareID                 View shared artifact (no auth)
```

Share links resolve on the render origin, under the artifact's own CSP. No account needed to view a share.

## Render surface

```
GET  /a/:artifactID    Serve artifact (render origin only)
GET  /s/:shareID       Serve shared artifact (render origin only)
```

The render surface sets `Content-Security-Policy` from the artifact's approved origins (its `decision='allow'` rows in `artifact_network_origins`), injects the storage shim with the artifact's state inlined, and serves the document `Cache-Control: no-store`. The iframe has `sandbox="allow-scripts"` without `allow-same-origin`, giving it an opaque origin.
