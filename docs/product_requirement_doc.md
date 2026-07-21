# Exhibit — Specification

A read-it-later library for self-contained web tools.

## 1. Objective

Provide the easiest way to **save, organize, find, share, and use** small,
self-standing HTML + JavaScript tools — the kind people generate with AI tools –
for a general audience.

The target artifact is a single-file tool that either:

- requires no persistence at all, or
- persists state through the browser's standard storage APIs (`localStorage`,
  `sessionStorage`, etc.).

Everything in this spec is in service of that objective. Anything that pulls the
product away from "it's just a file you can store forever and render anywhere" is
explicitly out of scope. This is like the user's home library. They have full control
over their artifacts.

## 2. Reference points

### Simon Willison's `tools.simonwillison.net`

The closest existing thing in spirit. It is a flat, statically-served collection of
self-contained HTML+JS tools, each its own page, with a colophon listing the build
transcript for every tool. It proves the core thesis: these tools are *files*, they
are durable, and they render identically years later because they carry their own
dependencies. Notable details that inform this spec:

- Tools are plain static pages — no backend, no per-tool server.
- Some tools persist state purely in `localStorage` (e.g. the event planner).
- He maintains tools that deal with exactly our security surface, including a
  CSP allowlist helper and an SVG sandbox for displaying untrusted markup safely.

What Simon's site *doesn't* do — and what this product adds — is the **library
layer**: collections, categories, full-text search, a rendered gallery,
and cross-device state for the tools themselves. His site
is a curated folder; this is the shelf, the index, and the sync.

### How the major providers handle it

- **Anthropic (Claude Artifacts):** dual-pane live rendering of HTML/CSS/JS/React in
  a sandboxed iframe, a version slider, and an injected async `window.storage` API
  that proxies persistence to a backend (per-artifact scope). Sharing exists and 
  share URLs are auth-gated.
- **OpenAI (ChatGPT Canvas):** side-panel document/code editor focused on inline
  editing and selection-scoped revisions, with a back-button version history. More
  editorial than rendering-oriented; weaker as a "build interactive thing" surface and
  no portable public artifact file.
- **Google (Gemini Canvas):** interactive previews plus deep Google Workspace / Docs
  integration. State and sharing are tied to the Google ecosystem.

The pattern across all three: the artifact lives *inside the assistant's walled
product*, its state lives on *that vendor's* backend, and getting the thing out as a
portable, re-runnable file ranges from awkward to impossible. This product's wedge is
that it treats the artifact as a portable file the user owns, decoupled from whoever
generated it.

## 3. Scope tiers

Three tiers exist. We build 1, add 2 if demand requires, and never build 3.

| Tier | What it is | Runs where | Portable as a file? | Decision |
|------|-----------|-----------|--------------------|----------|
| 1 | Static HTML/CSS/JS, optional `localStorage` | Visitor's browser | Yes | **Build now** |
| 2 | React/JSX via CDN imports (esm.sh, jsDelivr) | Visitor's browser | Yes | **Add on demand** |
| 3 | Backends, SSR, databases, server processes | A live server/sandbox | No | **Never** |

The line is drawn at "is it still just a file?" Tiers 1 and 2 are files: cheap to
store by the thousand, identical to render in five years. Tier 3 is a deployment — it
rots when a dependency or key expires, costs continuous compute, and breaks the
permanent-storage promise. Tier 3 artifacts are *apps* and belong on a deploy platform
(v0, Bolt, Replit, E2B-backed sandboxes), not in a library.

### Tier 2 specifics

React doesn't need a server; it needs a transform. Two options:

- **In-browser transpile** (Babel standalone, `type="text/babel"`): zero build infra,
  larger payload, slower first render. Matches how claude.ai renders React.
- **Build at ingest** (esbuild/Vite server-side, store the bundle): faster render, but
  adds a build pipeline so ingest is no longer a plain file copy.

External libraries come from CDNs via ESM imports (`import x from 'https://esm.sh/...'`),
not npm. The cost of tier 2 is that allowing CDN imports means the iframe must reach
the network — which is governed entirely by the security model in §6.

## 4. Architecture

### 4.1 Service-first, API as the single write path

Even though the product is local-first in feel, it is built as a hostable service from
day one to support later cross-device and multi-user use. The defining rule:

> **There is exactly one write path: the HTTP API.** Every ingest route — web upload,
> paste-in-browser — calls the API. Nothing writes the datastore directly.

This keeps every future feature (sync, sharing, multi-user) flowing through one
auditable mutation surface.

### 4.2 Components

```
┌─────────────┐     HTTP      ┌──────────────────────────┐
│  Web UI /   │ ────────────► │         Service          │
│  gallery    │               │  ┌────────────────────┐  │
│ (upload,    │               │  │  API (single write)│  │
│  paste)     │               │  └─────────┬──────────┘  │
└─────────────┘               │            │             │
┌─────────────┐               │  ┌─────────▼──────────┐  │
│  Chrome ext │ ────────────► │  │  Store interface   │  │
│  (future)   │               │  │  - metadata: SQLite│  │
└─────────────┘               │  │  - blobs: FS now,  │  │
                              │  │    S3-compatible   │  │
                              │  │    later           │  │
                              │  └────────────────────┘  │
                              │  ┌────────────────────┐  │
                              │  │ Render origin      │  │
                              │  │ (isolated host)    │  │
                              │  └────────────────────┘  │
                              └──────────────────────────┘
```

### 4.3 Storage abstraction

Define a backend-agnostic interface so the storage engine can evolve without touching
callers:

```
Store.put(artifact)        Store.get(id)
Store.search(query)        Store.list(collection)
Blob.put(id, bytes)        Blob.get(id)
```

- **Metadata / search / collections:** SQLite for v1. This is the deliberate on-ramp
  to later server-side durability and availability options (libSQL/Turso, rqlite,
  Litestream all build on SQLite without a data-layer rewrite). Note this is distinct
  from cross-device state, which is already solved by storing state on the server (§5).
- **Artifact bodies:** a blob interface, local filesystem now, S3-compatible later.

### 4.4 Schema (v1)

Carry forward-looking columns now even while single-user, because retrofitting them
is painful:

```sql
artifacts(
  id, owner_id,            -- owner_id hardcoded to 1 for now
  title, source_blob_id,
  source_url,              -- set when ingested by URL; enables re-fetch (§8.1)
  tier,                    -- 1 | 2
  created_at, updated_at,
  downloads_approved       -- first-use approval for the host-mediated download bridge
)
-- one decision per (artifact, origin), cascading with the artifact (§6)
artifact_network_origins(
  artifact_id, origin,
  decision,                -- 'allow' drives the render CSP; 'block' never does
  source,                  -- provenance: user | legacy | runtime
  created_at, updated_at
)
collections(id, owner_id, name)
artifact_collections(artifact_id, collection_id)
tags(id, owner_id, name, color)  -- name unique per owner
artifact_tags(artifact_id, tag_id)
artifact_state(artifact_id, key, value, updated_at)  -- the storage shim, §5
shares(id, artifact_id, public, expires_at)          -- sharing as a row, §7
```

### 4.5 Identity & auth (staged)

- **Now:** a single static token checked by middleware on every API call. `owner_id`
  always `1`.
- **Later:** real users behind the same middleware seam (a device-code/OAuth flow added
  without changing the underlying API contract).

## 5. Cross-device artifact state (the storage shim)

The hard problem isn't storing the artifact file — that's trivial and static. It's the
artifact's *own runtime state*: a todo list, saved config, typed notes. By default
that goes to `localStorage`, which is per-device and per-origin, so state set on iPhone
is invisible on Mac.

### 5.1 Principle: observe, don't predict

We do **not** analyze artifacts ahead of time to decide whether they use storage. No
static scan for this purpose, no LLM/agent inspection. Both are unreliable guesses at
something the runtime reports with certainty. Instead:

> Install a storage shim **unconditionally** before the artifact's scripts run. If the
> artifact uses storage, the shim captures it. If it doesn't, the shim sits idle at
> zero cost. There is no branch to decide.

### 5.2 What the storage shim intercepts

Before any artifact script executes, the iframe is initialized with **storage
adapters** — replacements for the standard storage surfaces that keep the
artifact-facing API identical and swap the backing to our server:

- `localStorage` and `sessionStorage` (synchronous) — **shipped in v1**
- `IndexedDB` (async/structured) — **deferred** (build-order step 2 remaining)
- `window.storage`-style async API (for artifacts written to the Claude contract) —
  **deferred** (build-order step 2 remaining)

The artifact uses whichever it was written for; we don't need to know which.

IndexedDB is a storage adapter by goal but **not** "the localStorage shim again."
The localStorage shim's defining trick is inlining all state synchronously at render
— necessary because `localStorage` is synchronous and read at startup before any
`await`, so an async fetch would lose the race (§5.3). IndexedDB is already
asynchronous and can hold large structured stores, so inlining neither applies nor
scales; it needs its own adapter design (likely lazy/bridged, not inlined).

### 5.3 Bridging sync APIs to a remote store

Two facts shape the solution: `localStorage` is **synchronous** and artifacts read it at
**startup** (before any `await`), while the network is async; and the artifact runs in a
**sandboxed, opaque-origin iframe** that cannot call the app API directly (its origin is
`null`, so every network call is cross-origin and CORS-blocked). Resolve each direction
separately:

1. **Reads — inline at render.** The render surface serializes this artifact's current
   state into the storage shim's in-memory cache **at request time**, embedded in the served
   document. So `getItem` is correct on the very first *synchronous* read, with no
   load-time fetch. (An async fetch-on-load instead races the artifact's own startup
   reads and loses — the artifact reads an empty cache before hydration lands.)
2. **Writes — bridge through the host frame.** `setItem` updates the cache synchronously,
   then posts the change to the **host frame** (the app-origin page embedding the iframe)
   via `postMessage`, pinned to the app origin. The host — same origin as the API and
   already authenticated — performs the write-through `PUT /api/artifacts/:id/state`. The
   sandbox never touches the network, so there is no CORS surface and the state endpoint
   stays authenticated (the single write path, §4.1, is preserved: the host calls the API).
3. Conflict policy: last-write-wins (adequate for this use case).

Result: iPhone writes land on the server; Mac reads them back — the second device's render
inlines the same state. Cross-device state with no skill, no special artifact format, and
no cooperation from the artifact's author.

### 5.4 Boundary

The storage shim only captures state crossing standard storage APIs. An artifact that persists
by `fetch`-ing its *own external* backend (Firebase, a private API) manages its state
elsewhere by definition — that's outside tiers 1/2 and we don't attempt to capture it.
Those network calls are instead governed by §6.

## 6. Security model

For tiers 1 and 2 the artifact runs **in the visitor's browser, not on the server** —
the service only stores and serves a file. So the model protects the visitor and, above
all, **controls and surfaces what the artifact may reach over the network.**

### 6.1 Isolation

- Serve artifacts from a **separate origin** (e.g. `artifacts.example.com`, ideally a
  per-artifact subdomain), never the app's own origin.
- Render in a sandboxed iframe. **Omit `allow-same-origin`** so artifact code cannot
  read the app's cookies, real-origin storage, or make authenticated same-origin
  requests. Add back only the minimum capabilities a tier needs.

### 6.2 Network control: scan, approve, allowlist, prompt

We do **not** use CSP violation reporting. Instead, a simpler explicit-consent flow,
with a per-artifact allowlist as the source of truth:

1. **Scan at ingest.** On `push`, statically parse the artifact for outbound
   references — `fetch(`/`XMLHttpRequest`, ESM/`import` URLs, `<script src>`,
   `<img src>`, `<link href>`, form actions — and extract the distinct origins.
2. **Show the footprint.** Present the user the list of origins the artifact will try
   to contact (fetch/post/import). Nothing is rendered with network access until they
   decide.
3. **Approve → allowlist.** Approved origins are written to the artifact's
   `artifact_network_origins` rows as `decision='allow'`. The render-time CSP `connect-src` / `script-src` / `style-src` /
   `img-src` / `font-src` are generated from this list; everything else is blocked by the
   browser. Default for a no-network tool is effectively `connect-src 'none'`. Inlined,
   no-egress assets are exempt from allowlist approval: `style-src 'unsafe-inline'` always
   permits inline `<style>`/`style=""`, and `img-src`/`font-src` always permit `data:`
   URIs (an inlined image or `@font-face` font is not a network request).
4. **Runtime escape → blocked.** If a rendered artifact attempts an origin **not** on
   its allowlist, the attempt is blocked by the browser's CSP. The user can approve
   the origin afterward in the artifact's allowlist editor, which updates the CSP on
   next render. A runtime approval prompt is tracked by `exhibit-fr7`; a "don't ask
   again" answer from it is stored as a `decision='block'` row, which suppresses the
   prompt and **never** affects the CSP.
5. **Per-artifact settings.** Decisions are visible and editable in each artifact's
   settings — the user can review, add, or revoke approved origins at any time, and
   blocked origins are listed separately so an earlier "don't ask again" stays visible
   and can be overridden rather than reading as merely undecided.

The static scan is **transparency, not a wall** (it's evadable). The enforced boundary
is the browser-level CSP generated from the allowlist; the scan just front-loads the
"approve these domains" decision so the common case needs no runtime interruption.

### 6.3 Residual risk

This controls what an artifact *reaches out to*. It does not stop a malicious artifact
from, e.g., rendering a convincing fake login form. The isolated-origin + no-
`allow-same-origin` setup contains the blast radius (it can't steal the real session).
If artifact sharing ever goes public/untrusted, inherit standard user-generated-content
hygiene. For single-user or trusted-circle use, not yet a concern — but the line is
known.

## 7. Sharing

Sharing is a first-class resource, not an export-to-file action.

- A share is a row: `shares(id, artifact_id, public, expires_at)`.
- Served at `GET /s/:shareId` with no auth, from the isolated render origin, under the
  artifact's own CSP allowlist.
- A one-file self-contained `.html` export is **planned** (CSS/JS already inline) — the
  portable fallback for email/Slack/offline that needs no service at all. Tracked in
  build-order step 3.

Because the artifact is already a portable file, sharing is nearly free; this is much
of what justifies hosting the service.

## 8. User flows

### 8.1 Capture (file from a CLI session)

The artifact is already a file on disk after a Claude Code / Gemini CLI session. No
skill or special output format is needed — the file *is* the artifact. It enters the
library through the web UI: drag/drop or paste the HTML, or paste a **URL** — the
service fetches the page once and stores it as an owned file (the URL is recorded as (inlining of relative assets is tracked by the open `exhibit-lwb` epic)
`source_url`, and the user can later re-fetch it on demand as a snapshot update — no
version history, with a warning that stored state may not survive the new body). On
ingest the service scans the file, surfaces its network footprint for approval (§6.2),
stores it, and returns the rendered/share URL.

### 8.2 Rediscover

Open the web gallery, search "bar chart" (matches indexed title; expanded to source + tags tracked by `av-b6o9`),
click the thumbnail, the tool renders live in its sandboxed iframe with its state
inlined from the service. No regeneration, no digging through chat logs.

### 8.3 Use across devices

Set state on iPhone → the storage shim posts to the host, which writes through to
the service → open the same artifact on Mac → the render inlines the state back
into the storage shim. Transparent to the artifact.

### 8.4 Share

One button mints a share row and returns `/s/:shareId`, openable by anyone in any
browser with no account and no dependency on the originating assistant — or (planned) export a
single self-contained `.html`.

## 9. Explicit non-goals

- **No tier-3 backends / no PaaS.** No running servers, SSR, or databases per artifact.
- **No live-linked imports.** URL-paste ingest exists (§8.1) but is a one-time
  vendoring fetch — after ingest the file is owned and served locally, never hot-linked
  or auto-synced.
- **No pre-render analysis step / no agent inspection** to detect storage usage — the
  runtime storage shim observes instead.
- **No CSP violation-report pipeline** — replaced by scan + explicit per-artifact
  allowlist with runtime permission prompts.

## 10. Build order

1. **Static core (tier 1):** service + single-write API, SQLite + blob store behind the
   Store interface, sandboxed iframe renderer on an isolated origin, ingest scan +
   allowlist + CSP generation, web gallery with search/tags/collections (upload + paste
   ingest).
2. **Storage shim:** unconditional `localStorage`/`sessionStorage` interception with
   render-time state inlining for reads and a `postMessage`-to-host bridge for writes
   (host performs the authenticated `PUT /api/artifacts/:id/state`); extend to IndexedDB
   and `window.storage`. Unlocks cross-device use.
3. **Sharing:** `shares` rows, public `/s/:id`, self-contained `.html` export.
4. **Tier 2 (only if demand):** in-browser Babel transpile first; CDN allowlist for
   `script-src`; ingest-time bundling only if render performance demands it.
5. **Multi-user (only if demand):** activate real identity behind the existing auth
   seam and `owner_id`. (Note: cross-*device* use needs nothing here — it is already
   handled in step 2 because all state lives on the server. SQLite replication is a
   separate, optional server-durability/availability concern; see §4.3.)

The static core and the eventual multi-user service share ~90% of the code; the only
real additions later are the auth middleware, a storage backend swap, and shares —
each made cheap by decisions taken in step 1.
