# Exhibit — Architecture

Companion to `product_requirement_doc.md` (the *what* and the boundaries) and
`technical_stack.md` (the *with what*). This document describes *how the
system is structured* — components, boundaries, data flow, and the request lifecycles
that matter. It assumes the decisions already made: Go service, single SQLite file,
blob store behind an interface, sandboxed-iframe renderer on a separate origin,
scan→allowlist→CSP network model, and the unconditional storage shim.

## 1. Architectural principles

Five rules shape every structural decision. When a choice is ambiguous, these decide it.

1. **It's just a file.** A tier-1/2 artifact is a self-contained document that runs in
   the *visitor's* browser. The service stores and serves it; it never executes artifact
   code. This is why the system stays small and why artifacts are durable.
2. **One write path.** Every mutation — upload, paste, future extension, state
   write-through — enters through the HTTP API. Nothing writes the datastore directly.
   This single seam is where auth, validation, and (later) replication and multi-user
   all attach.
3. **Two origins, hard boundary.** The app and the artifacts it renders live on
   different origins. The trust boundary between "our application" and "untrusted
   artifact code" is an origin boundary enforced by the browser, not a code convention.
4. **Observe, don't predict.** The system never analyzes an artifact ahead of time to
   guess its behavior (storage use, network use). It installs interceptors and policy at
   the runtime boundary and observes what actually happens.
5. **Easy path and serious path share one system.** Single-user/local and
   replicated/multi-user are the *same* binary and schema with optional pieces composed
   around them. No forks, no rewrites — seams placed early (owner_id, Store interface,
   single write path) make the upgrades additive.

## 2. System context

```
        ┌─────────────────────────────────────────────────────────┐
        │                      Deployer's host                      │
        │                                                           │
  ┌─────┴──────┐   TLS    ┌──────────────┐                         │
  │  Operator   │────────►│ Reverse proxy │  (operator-supplied,   │
  │   browser   │         │ (their choice)│   not shipped)         │
  └─────┬──────┘          └──────┬───────┘                         │
        │                        │ plain HTTP                       │
        │            ┌───────────┴────────────┐                     │
        │  APP_ORIGIN│                         │RENDER_ORIGIN        │
        │            ▼                         ▼                     │
        │   ┌─────────────────┐      ┌──────────────────┐           │
        │   │   App surface    │      │  Render surface   │          │
        │   │  (gallery, API)  │      │ (serves artifact  │          │
        │   │                  │      │  docs + CSP)      │          │
        │   └────────┬─────────┘      └─────────┬────────┘           │
        │            │   one Go process, two route groups            │
        │            └───────────────┬──────────┘                    │
        │                            ▼                               │
        │                  ┌───────────────────┐                     │
        │                  │  Store interface   │                     │
        │                  │  SQLite + blobs FS  │                     │
        │                  └───────────────────┘                     │
        │   optional, composed around: Litestream→bucket, thumb worker│
        └─────────────────────────────────────────────────────────┘

  Visitor's browser (may be a different person/device entirely):
   ┌──────────────────────────────────────────────────────────┐
   │  Page from RENDER_ORIGIN                                    │
   │   ┌────────────────────────────────────────────────┐       │
   │   │ sandboxed <iframe> (opaque origin)               │      │
   │   │   shim (localStorage→API)  +  artifact code       │     │
   │   │   network limited by per-artifact CSP             │     │
   │   └────────────────────────────────────────────────┘       │
   │            │ state read/write-through, over API             │
   └────────────┼──────────────────────────────────────────────┘
                ▼  back to App surface API
```

The same Go process answers both origins; they are route groups, not separate services.
The proxy that maps hostnames to the process and terminates TLS is the operator's, per
the tech-stack doc.

## 3. Components and responsibilities

### 3.1 API surface (the single write path)

The only way data changes. Route groups:

- `POST /api/artifacts` — ingest. Accepts a document body + metadata, **or a source
  URL** the service fetches once and stores as a file (the URL is persisted as
  `source_url`); runs the scan, returns the network footprint for approval, persists
  on confirmation.
- `GET /api/artifacts`, `GET /api/artifacts/:id` — list/detail (drives the gallery).
- `PATCH /api/artifacts/:id` — edits: title, body (rewrites the stored blob), tags,
  collections, `network_allowlist`.
- `POST /api/artifacts/:id/refetch` — for URL-ingested artifacts, re-fetches
  `source_url` and replaces the stored body. A snapshot, not a versioned update.
- `DELETE /api/artifacts/:id` — deletes the artifact, its blob, and associated state.
- `GET/PUT /api/artifacts/:id/state` — the storage shim's state endpoint (§6). Reads are
  normally satisfied by render-time inlining, not this route; `PUT` is called by the
  **host frame** on the shim's behalf (the sandboxed iframe can't reach the API itself).
  Authenticated like every other mutating route.
- `POST /api/shares`, `DELETE /api/shares/:id` — share lifecycle.
- collection/tag CRUD.

Middleware chain (via `chi`): request logging → auth (static token now, sessions later)
→ owner scoping (`owner_id`, fixed to 1 now) → handler. Auth and ownership are *one
layer* every mutating route passes through, which is what makes multi-user a
middleware-and-data change rather than a rewrite.

### 3.2 Render surface

A read-only surface on `RENDER_ORIGIN` whose entire job is to emit an artifact as an
executable document with the correct security envelope:

- Looks up the artifact, pulls its body from the blob store, its `network_allowlist`, and
  its current state.
- Generates the per-artifact CSP (`connect-src`/`script-src`/`style-src`/`img-src`/
  `font-src` from the allowlist) and sets it as a response header on the document.
  `connect-src` is the allowlist alone — the shim needs no network of its own (§6).
  Style/font defaults are permissive for *inlined* assets but strict for *network*
  ones, matching the "it's just a file" thesis: `style-src` always carries
  `'unsafe-inline'` (inline `<style>` blocks and `style=""` attributes never need
  network approval), and `img-src`/`font-src` always carry `data:` so an artifact
  that inlines its own images or fonts (`@font-face { src: url(data:…) }`) renders
  with zero network egress. Loading a stylesheet, image, or font *from a remote
  origin* still requires that origin on the allowlist — the network boundary is
  unchanged; only inlined, no-egress assets are permitted by default.
- Injects the **storage shim** as the first `<head>` script, with the artifact's state
  **inlined** into it so `getItem` is correct synchronously, then the artifact body.
- Sets `Cache-Control: no-store` — the document is dynamic (inlined state + per-artifact
  CSP) and must never be served stale from a cache.
- Is loaded by the app's pages as the `src` of a sandboxed iframe
  (`<iframe src="RENDER_ORIGIN/a/:id" sandbox="allow-scripts">`) with **no**
  `allow-same-origin`. The embedding page delegates clipboard access into the
  sandbox via Permissions Policy (`allow="clipboard-read; clipboard-write"`) so
  copy/paste works inside artifacts without weakening the origin isolation.

The render surface never mutates anything. It reads (including state, to inline it), wraps,
and serves. This read-only property is what makes it safe to expose under the no-auth share
path (§7).

### 3.3 Store interface

The seam between handlers and persistence. Handlers speak only to this interface:

```
Store:  put/get/list/search artifacts, collections, tags, shares; get/put state
Blob:   put/get artifact bodies by id
```

- **Metadata, collections, tags, shares, state** → SQLite (one file, WAL mode).
- **Search** → an FTS5 table over artifact titles. (Indexing source text and tag
  names to match the spec's search promise is tracked as av-b6o9.)
- **Bodies** → filesystem now, S3-compatible later — same `Blob` interface.

Because handlers never touch SQLite or the filesystem directly, swapping the metadata
engine (libSQL/Turso) or the blob backend (S3/MinIO) is a backend implementation change
behind a stable interface.

### 3.4 Ingest scanner

Invoked by `POST /api/artifacts`. Parses the document with a real HTML tokenizer
(`x/net/html`) to extract referenced origins (`src`/`href`/`action`/`<link>`/ESM
imports), plus a literal-URL heuristic over inline JS. Produces the deduplicated origin
list for the approval step. It is **transparency, not enforcement** — its output seeds
the allowlist; the CSP is the wall.

### 3.5 Gallery (web UI)

Server-rendered HTML emitted directly by Go handlers (`internal/api/gallery.go`),
styled with inline CSS and wired with small amounts of vanilla JS. Talks to the API
like any other client. Hosts two islands of client JS: the **CodeMirror** source
editor (an esbuild-built, `go:embed`-served bundle) and the **renderer iframe**
(which actually points at `RENDER_ORIGIN`). Everything else — search, filter,
tag/collection management, the allowlist editor — is full-page server renders.

### 3.6 Optional satellites (composed around, not shipped in)

- **Litestream** sidecar → streams the SQLite WAL to a bucket; supervises restore on
  empty volume.
- **Thumbnail worker** → headless Chromium screenshotting artifacts, kept out of the main
  image.
- **Future Chrome extension** → another API client for chat-UI ingest.

## 4. Trust boundaries

Four boundaries, in decreasing trust:

1. **Operator ↔ App API.** Authenticated (token now). The operator is trusted; this
   boundary is about identity and the single write path, not containment.
2. **App ↔ stored artifact body.** The body is untrusted data at rest. It is never
   executed server-side, never `eval`'d, only stored and later served. Treating it as
   inert bytes on our side is what keeps server-side risk near zero.
3. **Render origin ↔ visitor browser.** The artifact becomes *executing code* here — but
   in the visitor's browser, on a separate origin, inside an opaque-origin sandbox. The
   browser is the enforcer.
4. **Artifact code ↔ everything else.** The innermost and most important boundary. The
   sandbox (no `allow-same-origin`) + per-artifact CSP confine what artifact code can
   touch and reach. This boundary is *browser-enforced policy*, deliberately not our own
   code, because the browser's origin/sandbox/CSP machinery is far more battle-tested
   than anything we'd write.

The recurring theme: the hard security boundary is always pushed to the browser's native
mechanisms (origin isolation, iframe sandbox, CSP), because the server's best defense is
to never run artifact code at all.

## 5. Ingest data flow

```
client ──POST /api/artifacts (body + metadata, or a source URL)──► API
  API ──► if URL: fetch once (size-capped), body := fetched bytes, keep source_url
  API ──► scanner: tokenize, extract origins ──► footprint list
  API ──► respond: "these N origins will be contacted — approve?"
client ──confirm (+ edited allowlist)──► API
  API ──► Blob.put(body)         (untrusted bytes at rest)
  API ──► Store.put(artifact, network_allowlist, tier, source_url, ...)
  API ──► FTS5 index (title; see av-b6o9 for source + tags)
  API ──► respond: artifact id + render URL
```

Two-step by design: scan and surface *before* anything is renderable, so the network
footprint is a decision the user makes at the door, not a surprise at runtime. The
allowlist is **never seeded from the scan** — only origins the user explicitly
approves are written; until then the render CSP stays `connect-src 'none'`.

URL ingest is a *one-time vendoring fetch*, not a live link: the fetched document
becomes an owned file like any other artifact. The recorded `source_url` enables a
user-initiated `POST /api/artifacts/:id/refetch` that re-snapshots the body
(overwrite, no version history — and a warning that stored state may no longer fit
the new body).

## 6. Render + state data flow

```
visitor ──GET render URL──► Render surface
  Render ──► Store.get(artifact) + allowlist + state
  Render ──► build CSP from allowlist; compose <head>(shim, state INLINED) + body
  Render ──► Cache-Control: no-store; serve into sandboxed iframe
            (allow-scripts, NO allow-same-origin)

  iframe load (opaque 'null' origin — cannot call the API directly):
    artifact runs:
      getItem  ──► served synchronously from the inlined cache (no fetch, no race)
      setItem  ──► update cache sync; postMessage({k,v}) to the host frame
                     host (app origin, authed) ──► PUT /state (write-through, LWW)
    artifact fetch to origin X:
      on allowlist  ──► browser permits
      not on list   ──► browser blocks (CSP); UI prompts user → approve → PATCH allowlist
```

Two properties fall out of the sandbox's opaque origin: reads are **inlined at render**
(a load-time fetch would race the artifact's synchronous startup reads), and writes are
**bridged through the host frame** (the iframe can't call the API cross-origin, so the
authenticated host does it — no CORS, state endpoint stays authed).

The state endpoints are why cross-device "just works": all state lives server-side, so a
second device inlines the same state at render. No replication required for this (§8 distinguishes
it from server durability).

## 7. Sharing

A share is a row (`shares(id, artifact_id, public, expires_at)`), not an export action.
`GET /s/:shareId` resolves the row and serves the artifact **through the same read-only
render surface** under the same per-artifact CSP — just without the app auth check,
because the share row *is* the authorization. This reuse is why sharing is nearly free:
it's the render path with a different front-door check. A one-file self-contained `.html`
export remains as the service-independent fallback.

## 8. Evolution seams (how the easy path becomes the serious path)

Each future capability attaches to a seam already present in v1, so none is a rewrite:

| Future need | Attaches to | Change required |
|-------------|-------------|-----------------|
| Cross-device state | state endpoints (§6) | **already done** — state is server-side |
| Multi-user | auth middleware + `owner_id` | real sessions; scope queries by owner |
| Server durability / restore | Store (SQLite + WAL) | Litestream sidecar; no app change |
| HA / multi-region reads | Store interface | libSQL/Turso behind same interface |
| Object-storage bodies | Blob interface | S3/MinIO impl behind same interface |
| Tier-2 React | Render surface | add transpile (in-iframe Babel → esbuild) |
| Chat-UI ingest | API (single write path) | Chrome extension as a new client |

The point of the table: every column-3 change is *additive* and local, because the
column-2 seam was placed deliberately in the initial build. Cross-device, the thing most
likely to be confused for needing replication, needs nothing beyond what §6 already
specifies — server-side state is the whole mechanism.

## 9. What this architecture deliberately is not

- **Not a runtime/PaaS.** No tier-3 backends, no per-artifact server processes, no
  sandbox VMs. The moment an artifact needs a live server it stops being a file and
  leaves this system's scope.
- **Not a multi-service deployment.** One Go process answers both origins; SQLite is
  embedded; the only extra processes are optional satellites composed by the operator.
- **Not a predictor.** No pre-render static/LLM analysis gates behavior. Policy and
  interception sit at the runtime boundary and observe.
- **Not the owner of TLS or backup targets.** The release is the image plus a config
  contract (origins, data volume, optional Litestream env). Proxy, certs, and buckets are
  the operator's to compose.
