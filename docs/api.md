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
POST   /api/artifacts/:id/refetch  Re-fetch body from source_url (URL-ingested artifacts).
                                   400, artifact unchanged, when the source answers
                                   non-2xx, empty, or not HTML, or a saved asset
                                   cannot be refreshed. Body and assets commit
                                   atomically; storage failure leaves both unchanged.
                                   Never writes the allowlist; returns {artifact, updated,
                                   network_footprint, footprint_changed,
                                   undecided_origins, snapshot}
DELETE /api/artifacts/:id          Delete artifact, its associated rows, and its blobs
                                   (body + widget) — except a blob another artifact
                                   still references. 500 if a blob could not be removed:
                                   the row is already gone, but a delete that left bytes
                                   on disk must not report success
```

**Reclaiming bytes is automatic and invisible.** Rows and files live in two
stores that cannot commit together, so the transaction that removes the last
row naming a blob also records the intent to delete it (av-8gyd). The bytes go
immediately after that commit; anything a crash interrupts is finished by the
next startup. There is no sweep, no reconciler and no operator command — the
only thing a 500 above changes is when the file goes, not whether it does.

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

**Request size.** Every request body, on every route, is capped at
`MAX_REQUEST_BODY_BYTES`: 32 MiB unless the operator changed it
(`deployment.md` §2). A larger one gets a `413` before the server reads any of
it:

```json
{"error": "request body too large: this instance accepts at most 32.0 MiB per request",
 "limit_bytes": 33554432}
```

The cap counts the request as sent, JSON escaping included, so a pasted
document has a little less room than the figure. A body sent without a
`Content-Length` (chunked) is read up to the cap and then refused the same way.
A URL ingest is not bound by it: its request carries only the URL, and the page
the server fetches has its own 10 MiB cap, below.

**Storage quota.** When `ENTITLEMENTS_ENABLED` is on, artifact creates, body
rewrites, refetches and widget saves must fit the owner's resolved storage
allowance. Agent writes use the same gate. A refusal happens before storing
bytes and leaves the artifact, its metadata and its history unchanged:

```json
{"error":"storage quota exceeded: limit 100 bytes, current usage 90 bytes, refused write 20 bytes (20 additional bytes)",
 "code":"storage_quota_exceeded", "limit_bytes":100, "usage_bytes":90,
 "write_bytes":20, "additional_bytes":20}
```

The status is `413`. `write_bytes` counts the assembled content, including
snapshot assets; `additional_bytes` excludes assets the owner already stores.
The check uses current usage plus additional bytes. Historical versions retain
old bodies and widgets, so even a smaller rewrite adds its full byte length.
Snapshots are checked after vendoring; their request size cannot predict their
stored size. Saves that change no bytes and metadata-only edits need no space.
Reads, rendering, export and deletion remain available when over quota, and
lowering an entitlement never deletes artifacts. An unresolved entitlement or
unknown storage usage produces `500` and refuses the write. With enforcement
off, quota checks are bypassed.

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

## Stored-data imports

```
POST /api/artifacts/:id/state/import   Replace the owner's saved state from raw JSON
POST /api/artifacts/:id/assets/import Replace all current assets from an exported ZIP
```

Both routes are owner-only; recipients, anonymous visitors and scoped agent
credentials cannot import. State values must be strings, keys must be unique, and
an import may contain at most 10,000 keys. State replacement uses prepared inserts
under a five-second deadline. Asset ZIPs must belong to this artifact and include
`manifest.json` and exactly its listed payloads, retaining their asset IDs.

A nonempty target returns `409` with `confirmation_required: true` without
changing it. After the person confirms erasure, retry the same body with
`?replace=true`. Empty targets do not require confirmation. Success is `204`.
Replacement is atomic: omitted current entries disappear; an empty state object
or asset manifest clears the corresponding set. Invalid data and failed writes
preserve the original set. Other viewers' private state is untouched.

The global HTTP upload ceiling applies. ZIP limits are 200 assets, 96 MiB expanded
payloads, 48 MiB distinct payload bytes and a 1 MiB manifest. These limits cover
both markup and runtime rows for each fetched URL. Paths, metadata, CRCs and
payload sizes are validated before staging; archives are not extracted to disk.
An asset ID cannot change its bytes or MIME, even after deletion. Identical old
exports remain restorable. Blob quota checks measure the final replaced set,
retaining charges for payloads referenced elsewhere or by history. Invalid input
returns `400`; request, key-count, or storage limits return `413`.

See [the OWASP import review](stored-data-import-security-review.md) for security
coverage, durable identity records, and existing architecture/legacy limits.

## Versions

Every change to an artifact's body or widget — a `PATCH` that changes `body`, a
refetch, a widget `PUT`/`DELETE`, an agent write, a restore — is recorded as a
version. There is no route that creates one. A save that changes nothing
(identical body, identical widget, a title-only `PATCH`) records nothing.

A version carries the body and widget as they were and, once it has been
replaced, the saved data (`localStorage` state) the code had written by then —
snapshotted in the same transaction that created the next version.

`GET /api/artifacts/:id/versions` — newest first:

```json
{"versions": [
  {"artifact_id": "…", "seq": 3, "origin": "agent", "message": "make it count down",
   "session_id": "…", "has_state": false, "current": true, "created_at": "…"},
  {"artifact_id": "…", "seq": 2, "origin": "edit", "message": "", "session_id": "",
   "has_state": true, "current": false, "created_at": "…"}
]}
```

`origin` is `initial`, `edit`, `agent`, `refetch` or `restore`. `has_state` is
true for every version but the current one.

`POST /api/artifacts/:id/versions/:seq/restore` returns the artifact to that
version: its body and widget become the current ones and the saved data it left
behind replaces the live state, all as a **new** version (`origin: restore`) —
nothing in the history is discarded, and the state being replaced is snapshotted
first, so a restore can be undone. `409` when `:seq` is already the current
version or its source is no longer stored; `404` for an unknown version or an
artifact that is not yours. The allowlist and capability approvals are not
versioned and are not touched.

Both routes are owner-only and unavailable to an agent session's credential.

## Network origin decisions

```
GET    /api/artifacts/:id/origins            List every decision (allow and block)
POST   /api/artifacts/:id/origins            Decide one origin
                                             {"origin":"…","decision":"allow"|"block","source":"…"}
DELETE /api/artifacts/:id/origins?origin=…   Forget one decision (back to undecided)
```

`PATCH /api/artifacts/:id` with `network_allowlist` carries the whole approved
set and is what the edit page's Save uses. These routes decide a **single**
origin, and the difference is what they can express, not only how wide they are.
`PATCH` replaces the allow rows and deliberately leaves block rows alone, so it
can neither record a block nor return an origin to undecided; both are here.

- `decision: "allow"` widens the artifact's CSP on its next render.
- `decision: "block"` records a "don't ask again" answer. It **never** reaches
  the CSP; the render surface inlines it into the preamble purely so the runtime
  permission prompt stops raising that origin.
- `DELETE` is how a block is forgotten. Without it a block would be a permanent
  trap: a blocked origin never prompts again on its own.
- `source` is provenance and informational only, defaulting to `"user"`. The
  runtime prompt sends `"runtime"`.

The origin goes through the same normalization and the same `400` as a
`network_allowlist` entry (above), for the same reason: an allow row is pasted
verbatim into a CSP header. On `DELETE` it travels as a query parameter rather
than a path segment, because an origin is arbitrary caller-supplied text and a
path segment is resolved by the URL parser before the request is sent.

An agent-session credential cannot reach these routes at all: approving an
artifact's own network egress is a decision reserved for a person.

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

Each save is a new version of the artifact with a blob of its own (see
**Versions**); the widget URL is keyed by the artifact, so it is stable across
edits. `DELETE` removes the widget as a new version, and saving a tile identical
to the current one records nothing.

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

## Component kit (experimental, Exh-61ix)

```
GET /api/kit                                   Every released kit version, newest first
GET /api/kit/:version/components/:name         One component, with its stylesheet
```

`GET /api/kit` returns `{"latest":"v1","versions":[…]}`. Each version carries
its `marker` (the `<meta name="exhibit-kit" content="vN">` line an artifact
pins it with), `description`, `base`, `tokens` and `components` (name,
description, classes, usage example); the index carries no CSS. Both routes
are read-only and the same for every caller, so an agent session credential
may read them too. Unknown versions and components are `404`. See
[component-kit.md](component-kit.md).

## Agent (build/modify with AI, BYO key)

```
PUT    /api/agent/key                        Store provider settings {"provider","model","api_key","thinking_level"?} (key encrypted at rest)
GET    /api/agent/key                        Key status (masked hint only — the key is never returned)
DELETE /api/agent/key                        Remove the stored key
POST   /api/agent/sessions                   Start a session {"artifact_id"?: scope it to an existing artifact, "resume_session_id"?: continue a kept conversation of that artifact; omitted, the session is a new conversation} -> {"id","sse_ticket",…}
POST   /api/agent/sessions/:id/ticket        Mint a fresh SSE ticket for this session (the reconnect path)
POST   /api/agent/sessions/:id/prompt        Send a prompt {"message", "images"?: [{data, mime_type}], "snippets"?: [descriptor]}
POST   /api/agent/sessions/:id/abort         Abort the current run
DELETE /api/agent/sessions/:id               End the session (and drops its tickets)
GET    /api/agent/sessions/:id/events        SSE event stream (?ticket= auth — EventSource can't set headers)
GET    /api/artifacts/:id/transcripts        Conversations kept with an artifact -> {"head_seq", "transcripts": [{session_id, title, version_seq, resumable, updated_at}]}
GET    /api/artifacts/:id/transcripts/:sid   One conversation as what was said in it -> {…summary, "head_seq", "messages": [{role, text}]}
```

`thinking_level` accepts `off`, `minimal`, `low`, `medium`, `high`, `xhigh`,
or `max`. Empty uses the environment baseline (`AGENT_THINKING_LEVEL`, default
`medium`); omitted leaves an existing preference unchanged. GET returns the
saved `thinking_level` alongside the provider, model, and masked key hint.
An empty `api_key` keeps the stored key for the same provider, so changing
thinking or model does not require re-entering it. Invalid thinking levels
return `400`. These settings are per owner and apply when chats or widget
sessions start or resume. In hosted/platform mode all key/settings methods
return `404`, stored personal preferences are ignored, and only the environment
baseline is used.

A conversation is kept as Pi's own session file and tied to the artifact
version it was last working against (`version_seq`, read against `head_seq`);
only a projection of what a person saw is ever returned, never the file. A
resumable one can be continued with `resume_session_id` above (200 when it is
already running and the call attaches to it; 404 when it is not the caller's or
not there; 409 when it was kept before conversations could be resumed, or its
preceding process is still finishing persistence). Resuming
never changes the artifact — rolling it back is a separate
`POST /api/artifacts/:id/versions/:seq/restore`. See `docs/agent.md`.

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
captured inside the artifact — untrusted text the model reads as the result of
its `get_selection` tool rather than as part of `message`. See `docs/security.md` §5. The chat UI lives at `/agent`
(`/agent?artifact=<id>` to modify an existing artifact); snippet mode
(Ctrl+Shift+S) lets you click an element in the live preview and attach its
screenshot + selector to your next prompt. See [docs/agent.md](./docs/agent.md).

## Shares

```
POST   /api/shares                 Create anonymous link {"artifact_id":"..."}
POST   /api/shares                 Grant access {"artifact_id":"...","recipients":["alice"]}
GET    /api/artifacts/:id/shares    List the owner's anonymous link and grants
DELETE /api/shares/:id             Revoke an anonymous link (owner only)
DELETE /api/artifacts/:id/grants?recipient=:userID  Revoke a grant (owner only)
GET    /s/:shareID                 View anonymously shared artifact (render origin)
```

An anonymous link is a capability URL: anyone holding it can view the artifact without an account. It resolves on the render origin under the artifact's own CSP. Each artifact has at most one anonymous link; creating another returns `409`. Send `{"artifact_id":"...","replace":true}` to rotate it atomically, invalidating the old URL. A successful link create or replacement returns `201` with `share` (`id`, `artifact_id`) and `share_url`. Migration 039 preserves existing link ids and their URLs (Exh-grj9).

For restricted access, supply `recipients` with account login names or email addresses. This creates directed grants: recipients sign in and open the ordinary `/artifacts/:id` page. Grants have no ID; only `share_links.id` resolves at `/s/:shareID` (Exh-grj9). The response is `200` with a `results` entry per name (`granted`, `already_shared`, `unknown`, `ambiguous`, or `self`). `replace` is refused on grant requests. The store uses separate `CreateGrant` and `CreateAnonymousLink` paths; a grant requires a positive recipient ID, so a missing recipient cannot publish a link.

`public` is no longer a share field. Any create request carrying it, including `public: false`, `public: true`, or `public: null`, returns `400` before creating or replacing any link or grant. Omit it and use `recipients` when access should require a named account. Link responses omit `public` and `recipient_id`; grants are addressed by artifact and recipient.

`GET /api/artifacts/:id/shares` returns `link` (`id`, `url`) and `grants` (`recipient_id`, `name`, `disabled`). Grants expose no row ID. Revoke by artifact and recipient query parameter; a missing or invalid recipient is `400`. A successful revoke returns `204`; a missing grant, unknown artifact or non-owner returns `404`, matching link deletes.

`DELETE /api/shares/:id` removes only the anonymous link and leaves every grant intact. A grant's row ID returns `404`, just like an ID never issued; revoke grants only through the artifact and recipient route.

A share has no lifetime of its own: it is live from the moment it is minted until the row is deleted, using the applicable revoke route above. There is no expiry, and a create request carrying `expires_at` is refused with `400` rather than quietly given a link that never expires.

## Your own account

```
DELETE /api/account                {"confirm":"delete my library"}   → 204
```

Erases the caller's account and everything this instance holds for it: every artifact and its file (bytes, not only rows), all saved state, tags, collections, share links, the stored agent key and its transcripts, and the `users` row itself. It is permanent — there is no soft delete, no trash and no snapshot — and it revokes every share link over that library at once, for holders who have no account here and are not notified.

The route takes **no id**. It acts on the account the request's own session resolved to and cannot name another, which is why a session is the whole authorization for it; `/api/admin/*` is where acting on somebody else lives. A request carrying the service token instead of a session is answered `404`: that credential is not a person, and would otherwise resolve to the single-user default owner. The exact `confirm` phrase is required (`400` otherwise), and the instance's last enabled admin is refused (`409`) — promote somebody else first.

For self-hosted OIDC, deletion leaves the identity provider untouched. The same person signing in again gets a **new, empty** account.

For hosted Clerk accounts, deletion also removes the Clerk user through the backend SDK. Sessions are revoked first, and the local transaction records the identity deletion before the provider call. A provider failure after local erasure returns `202` with a JSON `message` explaining that sign-in account deletion is pending; the server retries at startup and every minute. Deleted identities are permanently blocked from recreating an Exhibit account. Deleting a Clerk user does not delete upstream Google, Apple, or other provider accounts. Clerk's own self-deletion control remains disabled so it cannot bypass Exhibit cleanup and the last-administrator guard.

## Render surface

```
GET  /a/:artifactID    Serve artifact (render origin only)
GET  /w/:artifactID    Serve the artifact's gallery widget (render origin only)
GET  /a/:artifactID/versions/:seq   Serve one earlier version, to look at (render origin only)
GET  /s/:shareID       Serve shared artifact (render origin only)
GET  /s/:shareID/widget  Serve a shared artifact's widget (render origin only)
GET  /s/:shareID/og.png  Link-preview image of a share: its widget, or the logo card
GET  /og.png           The generic link-preview card (Exhibit's logo)
```

A share document carries OpenGraph tags (`og:title`, `og:description`, `og:image`, `og:type`, `og:url`, `og:site_name`, `og:image:*`, `twitter:card`) naming the artifact and pointing `og:image` at `/s/:shareID/og.png`, a 1056x528 PNG of the share's widget. That route is `no-store`, and it serves the logo card when the instance runs no headless browser (`CHROMIUM_CDP_URL`) or the snapshot fails. Any id that is not a live anonymous link (a legacy grant id, a revoked link, or one never issued) gets the same `404` page, which carries only Exhibit's generic tags. See `security.md` §1.10.

The render surface sets `Content-Security-Policy` from the artifact's approved origins (its `decision='allow'` rows in `artifact_network_origins`), injects the storage shim with the artifact's state inlined, and serves the document `Cache-Control: no-store`. The iframe has `sandbox="allow-scripts"` without `allow-same-origin`, giving it an opaque origin.

`/a/:artifactID/versions/:seq` serves an earlier version's body beside the data that version left behind (what `POST …/versions/:seq/restore` would put back), so it can be looked at before it is restored. It takes its own token — scoped to that version and the owner, minted by the app origin's `/partials/version-viewer` fragment — so the artifact's own token does not open history and a version's does not open the live document. It writes nothing and nothing done in it persists — and when the tool in it tries to save, the document tells the host with an empty `__avVersionUnsaved` message (once per load; no op, key or value) so the page can say that nothing is saved until the version is the active one; the current version and anything not yours are `404`. See `security.md` §1.9.

`/w/:artifactID` is the same read path with the same CSP and the same inlined state — it differs only in which blob it serves and in getting the **narrowed preamble**: state writes stop at the in-memory cache and the capability bridges are not injected at all. It 404s for an artifact with no widget (`widgets.md`).

Frame state writes carry `X-Exhibit-Version: N`, the version sequence rendered
with their code. `PUT` and `DELETE` state routes check it atomically with the
mutation and return 409 (`code: "version_changed"`) for a superseded frame. This
prevents delayed writes from overwriting restored state. Explicit API edits and
the state inspector may omit the header.

## Conditional source and widget saves (av-n0gl)

Targeted edits may send `edits: [{"oldText":"unique existing text", "newText":"replacement"}]`
instead of `body` on artifact `PATCH` or widget `PUT`. These requests always
require the corresponding `expected_source_revision` or
`expected_widget_revision`, including for owner/API clients. Each match must be
exact and unique in the original document, and edits may not overlap or touch.
All replacements validate before any save. The normal scanning, versioning,
quota and atomic revision checks still apply. A widget patch returns confirmation
and its new revision without echoing the entire widget source. Full document
replacements continue to accept `body`; sending both forms is rejected.

Read `GET /api/artifacts/:id?body=true` for `body` and opaque
`source_revision` in the same response. Artifact JSON (including create and
metadata GET) also includes `source_revision` and `widget_revision`.
Read `GET /api/artifacts/:id/widget` for `body` and `widget_revision`.
An artifact with no widget still returns **404**, now with JSON
`{"error":"artifact has no widget","body":"","widget_revision":"…"}`.
An unknown artifact is a plain 404 with no revision; do not interpret it as an
empty widget. A missing/unreadable blob is also not an empty-widget snapshot.
Reads capture the pointer and token together; immutable version blobs keep
returned bytes paired with that token even when a save races the read.

Send the token from that read with the complete replacement document:

```json
{"body":"<html>…</html>","expected_source_revision":"…"}
```

on `PATCH /api/artifacts/:id`, or

```json
{"body":"<html>…</html>","expected_widget_revision":"…"}
```

on `PUT /api/artifacts/:id/widget`. PATCH success returns the saved source token
as `artifact.source_revision`; PUT success returns `widget_revision` alongside
`body`. These tokens identify **this request's saved content**, even if another
write has already advanced the head before the response arrives. Cache the
submitted bytes with that token, never a later head token. An identical save
keeps the token and creates no version, but still checks the precondition.

A stale source token returns **409** JSON with `code: "source_conflict"` and
`resource: "source"`; a stale widget token returns `code: "widget_conflict"`
and `resource: "widget"`. Both carry `error` instructing the caller to re-read
and retry. No content, bundled metadata, state or version changes commit on a
conflict; unused staged blobs are reclaimed. Keep the local buffer, explicitly
re-read, reconcile against the new bytes, and retry deliberately. There is no
automatic retry. A present token must be a nonempty string (otherwise 400).

Agent grants **must** supply the applicable token for source body PATCH and
widget PUT; omission returns **428** JSON `code: "precondition_required"`,
`resource: "source" | "widget"` and an explanatory `error`. Unconditioned agent tools
therefore fail safely. Built-in agent tools bind read revisions and send them
with every full-body write, including rewrites and widget creation. Conflicts
become re-read-and-retry tool errors without an automatic retry. Legacy owner/browser
and service-token API clients may omit the token for compatibility; such a
request uses the handler's current read as its precondition, protecting the
server read/commit interval only. It cannot protect a client's earlier stale
read. Built-in browser writers send explicit tokens from their read snapshots.
Title-only PATCH does not require a source precondition, but an explicitly
supplied precondition is always checked, even with an empty/ignored body.

`POST /api/artifacts/:id/refetch` accepts optional JSON
`{"expected_source_revision":"…"}`; a bodyless legacy request captures the
revision before fetching. Refetch compares that revision atomically on commit
and also checks it on an unchanged result. Success includes
`artifact.source_revision`. The detail page sends its displayed revision.
`DELETE /api/artifacts/:id/widget` accepts the raw widget token in `If-Match`
and returns 204 with `X-Widget-Revision` for the empty content. Agent grants
require that header; the widget panel always supplies it. A stale removal is
409 even if the current widget is already absent.

Tokens are hashes of immutable content pointers, independently for source and
widget; they are neither aggregate history sequence numbers nor timestamps.
Widget/state/metadata changes leave an unchanged source's token valid, and
source edits leave an unchanged widget's token valid. Restores reuse the saved
pointers, so restoring the exact previous content identity makes its old token
valid again (ABA is intentionally content-based). A restore that only changes
state does not invalidate source. An identical rewrite preserves identity;
rewriting different bytes mints a fresh identity even within the same second.
Do not derive tokens from bytes, parse them or use them to order versions.
