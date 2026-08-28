# Exhibit — API Reference

Companion to `product_requirement_doc.md` (the *what* and the boundaries) and
`architecture.md` (the *how* — see §3.1, the API surface as the single write
path). This document is the concrete HTTP API: every route, its auth, and the
ingest, state, sharing, and render flows a client uses.

All routes require `Authorization: Bearer <token>` except public share links.
An agent session's own bearer token is accepted too, but only within its scope
(see Agent, below).

## Artifacts

```
POST   /api/artifacts              Ingest an artifact (inline body, or url to fetch once)
GET    /api/artifacts              List artifacts (?q=search&tags=a,b&collections=c)
GET    /api/artifacts/:id          Get artifact metadata (?body=true for source)
PATCH  /api/artifacts/:id          Update title, body, network_allowlist, etc.
                                   (network_allowlist is the whole approved set; it
                                   replaces the artifact's allow decisions and leaves
                                   any blocked origins untouched)

Every `network_allowlist` entry — on `POST` and on `PATCH` alike — must be an
**origin**: an absolute `https://host[:port]` (plaintext `http://` only for
loopback hosts). Scheme and host are lowercased, a trailing dot on the host is
stripped, a default port is dropped, and duplicates collapse. Anything else —
a URL with a path or query, credentials, a wildcard, a CSP keyword, a
`data:`/`blob:` source — is a `400` naming the offending value, not a silently
truncated row: a path-bearing entry approved as one file would otherwise grant
its whole origin (av-i7hd).
POST   /api/artifacts/:id/refetch  Re-fetch body from source_url (URL-ingested artifacts)
DELETE /api/artifacts/:id          Delete artifact, its associated rows, and its blobs
                                   (body + widget). 500 if a blob could not be removed:
                                   the row is already gone, but a delete that left bytes
                                   on disk must not report success
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

Binary payloads the page fetches **from JavaScript** at runtime — a wasm module,
an Emscripten `.data` heap — are vendored too, under a larger per-asset cap. This
is what makes such tools work at all: once relocated to the render origin, a
fetch that was same-origin on the source site becomes cross-origin, and source
sites do not send CORS headers for requests that never needed them. CSP permits
the request; the browser refuses to *read* the response, so the failure looks
like a network error that **approving the origin cannot fix**. Inlining removes
the request. These assets are matched by extension (`.wasm`, `.data`, `.bin`,
`.mem`) and served through an injected `fetch` wrapper, so a URL the page builds
at runtime is still satisfied locally.

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
GET    /api/artifacts/:id/state       Get all state key-value pairs
PUT    /api/artifacts/:id/state         Set one key {"key":"...","value":"..."}
DELETE /api/artifacts/:id/state?key=K   Remove one key's row
DELETE /api/artifacts/:id/state         Erase every state row you hold on the artifact
```

State rows are keyed by `(artifact, viewer, key)` (av-q0ub), and every route
here addresses **your own** rows: a `GET` returns your state, not the union of
every viewer's, and the second `DELETE` erases yours, not the artifact's.
"Viewer" is the authenticated session — one person across any number of devices,
which is what makes the sync below work; there is nothing device-shaped in the
request.

These routes back `localStorage` only. The storage shim intercepts it in the iframe: reads are served from state **inlined into the shim at render time** (so `getItem` is correct synchronously); writes are **`postMessage`-ed to the host frame**, which performs the authenticated `PUT` above (the sandboxed iframe has an opaque origin and can't call the API itself). No artifact changes needed — any tool that uses `localStorage` gets cross-device sync automatically.

`sessionStorage` is intercepted too, but into a separate in-memory namespace that never touches these routes — it is frame-local by design and produces no state rows (`security.md` §1.2).

One `DELETE` route serves both operations, discriminated by whether a `key`
parameter is **present** — not by its value, since `""` is a legitimate Web
Storage key that must stay deletable on its own. `?key=` (present, empty)
removes the empty-string key; no `key` at all erases everything.

The key is a **query value rather than a path segment**, deliberately. As a
segment it was subject to URL normalization in the browser *before the request
was sent*: a key of `..` turned `DELETE /api/artifacts/:id/state/..` into
`DELETE /api/artifacts/:id/`, letting an artifact destroy itself through the
host frame's token by calling `localStorage.removeItem('..')` (av-hh1o). A
query value has no segment structure to resolve. It also means the empty-string
key is representable at all, and that a long key can't overflow the request line
on delete when the `PUT` body would have carried it fine. Clients still
percent-encode the value (`encodeURIComponent`).

Both deletes answer `204` and are **idempotent**: a key that was never stored is
already absent, which is all the caller asked for. They 404 only on an unknown
artifact. Erasing all state touches state alone: the artifact's body, origin
decisions, and capability approvals are untouched.

Deleting is a genuine row removal. The shim's `removeItem` drives this route, so
a removed key is absent on the next render rather than reading back as `""` —
early builds wrote an empty-string tombstone instead (av-ms3r), and rows written
by that code may still exist; they are indistinguishable from an intentional
`""` and are left for the user to clear by hand. The edit page's state inspector
(`/artifacts/:id/edit`) is the user-facing consumer of these routes: it renders
each stored value through a control inferred from its shape and never offers
raw-text editing of a value.

## Gallery widget

```
GET    /api/artifacts/:id/widget            Read the widget source (404 when there is none)
PUT    /api/artifacts/:id/widget            Save/replace it {"body":"<html>…"}
DELETE /api/artifacts/:id/widget            Detach it; the card falls back to the default tile
POST   /api/artifacts/:id/widget/generate   Have the agent write it
```

A widget is the small, informative tile an artifact's gallery card renders
(`av-fafu`; see `widgets.md`). It is a second document under the *artifact's*
security envelope, not a resource of its own: it reads the same state, renders
under the same CSP allowlist, and can write nothing.

`PUT` responds with the saved body plus transparency about its references:

```json
{
  "body": "<html>…",
  "network_footprint": ["https://cdn.example.com"],
  "unapproved_origins": ["https://cdn.example.com"],
  "widget_url": "https://artifacts.example.com/w/…"
}
```

`unapproved_origins` are the footprint origins the **artifact's** allowlist does
not cover. Unlike an ingest footprint these are not awaiting approval — they are
already blocked at render — so the field exists to explain a blank tile, not to
gate one. As everywhere else, the scan never seeds the allowlist.

The widget's blob id is minted once and reused on every later save, so an
artifact's widget URL is stable across edits.

`POST …/widget/generate` starts a one-shot agent session scoped to writing this
artifact's tile and returns immediately:

```json
{ "session_id": "…", "sse_ticket": "…" }
```

It takes **no body** — the prompt is a server-side constant and the scoping is
in the session's system prompt, so a caller cannot steer the model through this
route. It does not wait for the result either: subscribe to the session's
ordinary stream (`GET /api/agent/sessions/:id/events?ticket=<sse_ticket>`) and watch for
`exhibit_widget_saved`, then `DELETE /api/agent/sessions/:id`. Returns `503`
when the `pi` binary is absent and `412` when no provider key is configured.

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
POST   /api/agent/sessions                   Start a session {"artifact_id"?: scope it to an existing artifact} -> {"id","sse_ticket",…}
POST   /api/agent/sessions/:id/ticket        Mint a fresh SSE ticket for this session (the reconnect path)
POST   /api/agent/sessions/:id/prompt        Send a prompt {"message", "images"?: [{data, mime_type}], "snippets"?: [descriptor]}
POST   /api/agent/sessions/:id/abort         Abort the current run
DELETE /api/agent/sessions/:id               End the session (and drops its tickets)
GET    /api/agent/sessions/:id/events        SSE event stream (?ticket= auth — EventSource can't set headers)
GET    /api/artifacts/:id/transcripts        Agent conversations persisted with an artifact
```

Every `sse_ticket` is session-bound, single-use, and valid for 30 seconds
(av-rgp1; full contract: `architecture.md` §3.7, `security.md` §6). A client
must mint a fresh one for each connect — including a reconnect — rather than
caching or reusing one.

Each session spawns a [Pi](https://github.com/badlogic/pi-mono) sidecar
(`pi --mode rpc`) whose only tools call back into this API, so agent output
enters the library through the same ingest path (scan + explicit allowlist
approval) as everything else. The sidecar authenticates with a **per-session
credential scoped to one artifact**, not the service token: it may
`POST /api/artifacts` until it binds, then `GET`/`PATCH` that artifact and
`GET`/`PUT`/`DELETE` its `state` and `widget` sub-resources — one entry per
tool it has — and every other route answers 403. Its owner comes from the
same credential, so the owner-scoped store calls bound it to one tenant on top
of that. `snippets` entries are element descriptors
captured inside the artifact — untrusted text the server fences as data rather
than splicing into `message`. See `docs/security.md` §5. The chat UI lives at `/agent`
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

A share has no lifetime of its own: it is live from the moment it is minted until the row is deleted, and `DELETE /api/shares/:id` is how it ends. There is no expiry (av-8ipt removed a column nothing ever set), and a create request carrying `expires_at` is refused with `400` rather than quietly given a link that never expires.

## Your own account

```
DELETE /api/account                {"confirm":"delete my library"}   → 204
```

Erases the caller's account and everything this instance holds for it: every artifact and its file (bytes, not only rows), all saved state, tags, collections, share links, the stored agent key and its transcripts, and the `users` row itself. It is permanent — there is no soft delete, no trash and no snapshot — and it revokes every share link over that library at once, for holders who have no account here and are not notified.

The route takes **no id**. It acts on the account the request's own session resolved to and cannot name another, which is why a session is the whole authorization for it; `/api/admin/*` is where acting on somebody else lives. A request carrying the service token instead of a session is answered `404`: that credential is not a person, and would otherwise resolve to the single-user default owner. The exact `confirm` phrase is required (`400` otherwise), and the instance's last enabled admin is refused (`409`) — promote somebody else first.

Deleting here cannot touch the identity provider that issued the login. The same person signing in again gets a **new, empty** account, because `external_id` is unique and the row is created at first login.

## Render surface

```
GET  /a/:artifactID    Serve artifact (render origin only)
GET  /w/:artifactID    Serve the artifact's gallery widget (render origin only)
GET  /s/:shareID       Serve shared artifact (render origin only)
```

The render surface sets `Content-Security-Policy` from the artifact's approved origins (its `decision='allow'` rows in `artifact_network_origins`), injects the storage shim with the artifact's state inlined, and serves the document `Cache-Control: no-store`. The iframe has `sandbox="allow-scripts"` without `allow-same-origin`, giving it an opaque origin.

`/w/:artifactID` is the same read path with the same CSP and the same inlined state — it differs only in which blob it serves and in getting the **narrowed preamble**: state writes stop at the in-memory cache and the capability bridges are not injected at all. It 404s for an artifact with no widget (`widgets.md`).
