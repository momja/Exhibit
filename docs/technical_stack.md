# Exhibit — Technology Specification (Supplemental)

Companion to `product_requirement_doc.md`. That document defines *what* to build and the
boundaries; this one recommends *how* — concrete technologies, with the reasoning and
one credible alternative per choice so the decisions are yours, not assumed.

The through-line for every recommendation is the product's deployment promise: a
**single, easy-to-self-host service** that ships as a small Docker image, stores
everything in one place, and degrades gracefully from "one command" to "replicated for
safety" (§12).

## 1. Stack at a glance

| Layer | Recommendation | Main alternative |
|-------|---------------|------------------|
| Service language | Go | Python + FastAPI |
| HTTP routing | stdlib `net/http` (1.22+) + `chi` middleware | Echo / Fiber |
| Metadata DB | SQLite | — (keep it) |
| SQLite driver | `modernc.org/sqlite` (pure Go, no CGO) | `mattn/go-sqlite3` (CGO) |
| Search | SQLite FTS5 | Bleve / external (avoid) |
| Migrations | `goose` | `golang-migrate` |
| Blob store | Local FS behind interface → S3/MinIO later | — |
| Source view/edit | **CodeMirror 6** | Monaco (heavier) |
| Artifact renderer | Sandboxed `<iframe>` + per-artifact CSP | — |
| Tier-2 transpile | Babel standalone (in-iframe) → `esbuild` later | SWC |
| Storage shim | Vanilla JS, bundled with `esbuild` | — |
| Ingest scan | `x/net/html` parser (+ JS heuristic) | — |
| Thumbnails | Headless Chromium worker (`chromedp`) — optional | client `html2canvas` |
| Gallery UI | Server-rendered stdlib `html/template` + static CSS/JS assets (§9) | templ (codegen — rejected) |
| Agent harness | **Pi** (`pi --mode rpc` sidecar per session; TS tools extension; keys AES-GCM at rest; `cmd/mockllm` for tests) | Claude Agent SDK (heavier, vendor-tied) |
| Icons | **Phosphor Icons** — self-hosted / embedded on app origin, no CDN (§9) | Lucide / Heroicons |
| TLS / proxy | **Operator's choice** — app serves plain HTTP, takes origin config | (not shipped) |
| Backup/replication | Litestream sidecar (Compose profile) | Turso/libSQL (HA) |

## 2. Service language & framework

**Decided: Go.** The product's entire value rests on being trivially self-hostable, and
Go fits that goal precisely: it compiles to one static binary with no runtime, which
becomes a ~15–25 MB `scratch`/`distroless` image; its concurrency model handles the
frequent small state write-through calls without an async framework; and the SQLite +
FTS5 + blob design has mature, dependency-light Go support. (Python + FastAPI would have
ported the architecture cleanly too, at the cost of a heavier image and a process
manager — noted only to record the road not taken.)

Routing: with Go 1.22+ the stdlib mux covers method+path routing; add `chi` only for
clean middleware chaining (auth, logging, the single-write-path guard). Avoid heavier
frameworks — there's not enough surface area to justify them.

## 3. Data layer

**SQLite, kept as-is.** It is the correct primary store for this product, not a
placeholder: one file, embedded, no separate service, and the on-ramp to every later
durability option (§12).

**Driver: `modernc.org/sqlite` (pure Go).** No CGO means trivial cross-compilation and
the smallest possible Docker image. It supports FTS5, which you need. Switch to the CGO
`mattn/go-sqlite3` only if you later measure search/write performance that demands it —
unlikely at this product's scale.

**Search: SQLite FTS5.** A single external-content FTS5 table delivers the gallery's
search with zero extra infrastructure. It indexes artifact title, source text, and tag
names — a single search box query matches any of the three.

**Migrations: `goose`.** Embed migration files in the binary (`go:embed`) and run them on
startup so a fresh container self-initializes.

**Blob store: filesystem behind the `Blob` interface now.** Artifact bodies are written
to a mounted volume; later, an S3-compatible backend (AWS SDK v2 or `minio-go`) drops in
behind the same interface. For self-hosters who want object storage, a MinIO container is
the natural local S3 (offer it as a Compose profile, §12).

WAL mode on from day one (`PRAGMA journal_mode=WAL`) — better concurrency and the
prerequisite for Litestream.

## 4. The renderer

This is the core of the product and it is **not** CodeMirror — keep the two ideas
separate:

- **Running the tool** = a sandboxed `<iframe>` that executes the artifact's HTML/JS.
- **Showing the code** = CodeMirror (§5).

Renderer construction:

- Serve the artifact document from the isolated render origin and point the iframe's
  `src` at it (`RENDER_ORIGIN/a/:id`; §12 covers the origin/TLS implications).
- `sandbox="allow-scripts"` — and deliberately **omit `allow-same-origin`**, putting the
  iframe in an opaque origin. This is what prevents an artifact from touching the app's
  cookies/storage and what lets two artifacts coexist without reading each other, even on
  a shared render origin.
- The embedding page grants `allow="clipboard-read; clipboard-write"` on the iframe —
  a Permissions Policy delegation so artifacts can use the async Clipboard API without
  any relaxation of the sandbox or CSP.
- Inject a generated **per-artifact CSP** (`connect-src`/`script-src`/`style-src`/
  `img-src`/`font-src`/`media-src` built from the artifact's allowlist) into the served
  document. The browser enforces the network boundary; this is the wall behind §6 of the
  main spec. Inlined/local assets are exempt from the allowlist since they carry no
  network egress: `style-src` always carries `'unsafe-inline'`, `img-src`/`font-src`
  always carry `data:`, and `media-src` always carries `blob:`, so inline styles,
  inlined `data:` images/fonts, and a locally imported file played back via
  `<video>`/`<audio src=blob:...>` (`URL.createObjectURL` on a picked `File`) all
  render without approval.
- Inject the **render preamble** — the **storage shim** (§6 here) with the artifact's
  current state inlined — into `<head>` *before* any artifact script runs.
  Serve the document `Cache-Control:
  no-store` — it's dynamic (inlined state + per-artifact CSP) and must not be cached.

**Tier 2 (React via CDN), when demand arrives:** start with Babel standalone loaded
inside the iframe and `type="text/babel"` scripts — zero build infrastructure, matches
how claude.ai renders React. Move to a build step (`esbuild`, or `esbuild-wasm` in the
browser) only if first-render latency from in-iframe Babel becomes a real complaint. CDN
imports (`esm.sh`, `jsDelivr`) are governed by the same `script-src` allowlist — decide
whether tier-2 artifacts ship with those CDN origins pre-seeded or surfaced for approval
at ingest.

## 5. CodeMirror — source view and editing

**CodeMirror 6** is the right call for displaying and editing artifact source. To be
precise about its role: CodeMirror renders the *code* (syntax-highlighted, editable
HTML/CSS/JS), while the iframe renders the *running tool*. A typical artifact detail view
shows both side by side — CodeMirror on one side, live iframe on the other — which also
gives you a clean edit→re-render loop.

Modules to pull in:

- `codemirror` (the meta-package) or assemble from `@codemirror/state` + `@codemirror/view`
- `@codemirror/lang-html`, `@codemirror/lang-javascript`, `@codemirror/lang-css`
- `@codemirror/commands`, `@codemirror/search` for editor UX
- a theme (e.g. `@codemirror/theme-one-dark`)

CodeMirror 6 is modular and tree-shakeable, so it stays light. Prefer it over Monaco
here: Monaco is excellent but ships a much larger bundle and a VS Code-grade feature set
you don't need for viewing/lightly editing self-contained files.

Bundle CodeMirror (and the gallery's JS) with `esbuild` into a static asset the Go binary
serves via `go:embed`. No Node runtime in production — Node is a build-time-only
dependency.

Lint the editor source with ESLint (flat config, `@eslint/js` recommended rules) before
it's bundled: `cd web/editor && npm run lint`. The config mirrors the esbuild target
(es2020, ES-module, browser globals).

## 6. The storage shim

Plain **vanilla JavaScript**, no framework — it must run before anything else in the
iframe and stay tiny. Bundle it with `esbuild` and inject it as the first `<head>`
script.

Responsibilities (per the main spec §5):

- Replace `localStorage`/`sessionStorage` with objects implementing the `Storage`
  interface, backed by an in-memory cache. The cache is **inlined into the storage
  shim by the render surface** at request time, so `getItem` is correct on the first *synchronous*
  read (a load-time `fetch` would race the artifact's startup reads and lose).
- On `setItem`, update the cache synchronously, then **`postMessage` the change to the
  host frame** (pinned to the app origin). The host — same origin as the API and
  authenticated — performs the `PUT /api/artifacts/:id/state`. The storage shim itself makes
  **no network calls**: the sandbox's opaque origin can't reach the API cross-origin, so
  it never has to, and `connect-src` need not include the app origin.
- `IndexedDB` interception and the `window.storage`-style async API are **deferred**
  (build-order step 2 remaining). v1 ships `localStorage` and `sessionStorage` only.
- Last-write-wins on conflicts.

Keep this as a single audited file — it's security-sensitive (it sits between untrusted
artifact code and your API) and should be easy to read end to end.

## 7. Ingest scan

Purpose is **transparency, not enforcement** (the CSP is the wall). On ingest, extract
the outbound network footprint to show the user for approval.

- Parse HTML with `golang.org/x/net/html` (a real tokenizer, never regex) to collect
  origins from `src`, `href`, `action`, `<link>`, and ESM import URLs.
- For JS `fetch`/`XMLHttpRequest` targets, accept that full static analysis is
  impossible — use a lightweight heuristic pass (string/AST scan via esbuild's parser for
  literal URLs) and clearly treat anything it finds as a hint. Whatever it misses is
  caught at runtime by the CSP allowlist + permission prompt.

Present the deduplicated origin list at the approval step; write approved origins as
the artifact's `decision='allow'` rows in `artifact_network_origins`.

## 8. Thumbnails (optional, defer if needed)

For the gallery grid, the high-fidelity approach is a **headless Chromium worker** that
loads the artifact and screenshots it. In Go, `chromedp` drives Chromium over CDP; run it
as a separate worker/sidecar so the main service image stays slim, since bundling
Chromium adds ~several hundred MB.

Lighter alternative: render the artifact in a hidden iframe client-side and capture with
`html2canvas` — no Chromium, but imperfect fidelity (it re-rasterizes the DOM rather than
truly rendering). 

This is a nice-to-have; a v1 can ship with a generated placeholder (favicon/title card)
and add real thumbnails later without schema changes.

## 9. Gallery UI

**As built: server-rendered pages via the stdlib `html/template`** — templates in
`internal/api/templates/` (committed source, `go:embed`-ed), handlers and view models
in `internal/api/gallery.go` (epi-q0u2). Each page's CSS and JS are static assets
authored in the `web/gallery/` workspace, copied into the embedded assets at build
time (§13), and served under `/assets/gallery/`; per-request values reach the page
scripts through a small inline bootstrap `<script>` that html/template JSON-encodes.
The gallery is CRUD-shaped — grid, search (eager client-filtered by swapping the
server-rendered grid), tag/collection filters, a detail view — and full-page server
renders cover it, keeping everything inside the one Go binary with no frontend
framework and no template codegen. (templ — the codegen engine an early scaffold
used — was considered for the extraction and rejected: the stdlib engine adds zero
dependencies and no generate step, and its contextual auto-escaping replaces the
hand-rolled HTML escaping the old string-concatenated pages needed.)

CodeMirror and the renderer iframe are islands of client JS inside these
server-rendered pages.

**Icons: Phosphor Icons — the required icon set for all new UI.** Standardize on
[Phosphor Icons](https://phosphoricons.com) so every future story inherits one consistent
icon vocabulary without re-deciding. Load it **self-hosted on the app origin, never from a
third-party CDN** — consistent with this project's self-contained, `go:embed`-ed static
asset stance (§12–§13) and the two-origin security model (icons ship with the app surface,
not the render origin). Vendor the `@phosphor-icons/web` package at build time, bundle its
CSS + webfont into the embedded assets, and serve them from the app origin. Icons are then
plain markup the server-rendered pages emit directly — no client JS, no runtime fetch:

```html
<!-- Load once in the app shell's <head>, from our own origin: -->
<link rel="stylesheet" href="/assets/phosphor/regular.css">

<!-- weight = the class family: ph (regular), ph-bold, ph-fill, ph-duotone, ph-thin, ph-light -->
<i class="ph ph-magnifying-glass"></i>   <!-- search -->
<i class="ph-bold ph-trash"></i>          <!-- delete, bold weight -->
```

If you prefer inline SVG (crisper sizing control, no webfont), vendor the same icons as an
embedded SVG sprite and reference symbols by id — still served from the app origin, still
no CDN:

```html
<svg class="icon" width="20" height="20" aria-hidden="true"><use href="/assets/phosphor.svg#magnifying-glass"></use></svg>
```

Either path, the rule is fixed: **Phosphor Icons, self-hosted, no external icon CDN.**

## 10. Auth

- **Now:** a single static bearer token checked by `chi` middleware; `owner_id` fixed at
  `1`. Sufficient for single-user/self-host.
- **Later:** signed-cookie sessions (or a small library equivalent) behind the same
  middleware seam, plus a login flow — no change to the API contract or data model
  because `owner_id` and the auth boundary already exist.

## 11. Future: Chrome extension

For importing artifacts that live inside chat UIs (claude.ai, ChatGPT) rather than on
disk: a Manifest V3 extension with a content script that reads the rendered artifact's
HTML from the page DOM and `POST`s it to `/api/artifacts`. The service API must allow the
extension's origin via CORS. This is the eventual answer to the browser-chat ingest gap
and replaces any need for a CLI tool.

## 12. Deployment

**Image:** multi-stage Docker build — build with the Go toolchain (and Node only to
bundle CodeMirror/gallery JS), copy the single binary + embedded assets into
`distroless`/`scratch`. One small image, one process.

**TLS / reverse proxy: the operator's, not ours.** We don't ship or require a proxy.
The service serves plain HTTP on a bound port; whatever sits in front (Caddy, Traefik,
nginx, a cloud load balancer) terminates TLS and routes hostnames. The product's only
requirement here is an origin-separation one, expressed as config:

- The app reads `APP_ORIGIN` and `RENDER_ORIGIN` (e.g. `https://artifacts.example.com`)
  and builds all artifact URLs, share links, and per-artifact CSP from `RENDER_ORIGIN`.
  Serving artifacts from a **different origin** than the app is what the security model
  needs (§4); how that origin resolves to a host and cert is the deployer's setup.
- *Baseline (simple):* point two hostnames at the container — the app and one render
  origin. Combined with opaque-origin sandboxed iframes (§4), this already isolates
  artifacts from the app and from each other. No wildcard cert needed.
- *Hardened (optional):* per-artifact subdomains `<id>.artifacts.example.com` for
  defense-in-depth. The service routes them if the operator points wildcard DNS + a
  wildcard cert at it — but provisioning that wildcard is the operator's job, not part of
  any release. Baseline is enough for single-user/trusted use.

Document the *requirement* ("serve the app and render origin as two hostnames; terminate
TLS however you like") and include a sample proxy snippet or two as a convenience clearly
labeled as examples — not as a shipped component.

**Backup / replication via Compose (the §earlier discussion):** the app always opens the
same plain SQLite file; replication is a sidecar, selected at deploy time.

```yaml
services:
  app:
    image: exhibit
    volumes: [ data:/data ]            # opens /data/app.db in both modes
    environment:
      - REPLICATION=${REPLICATION:-off}

  litestream:
    image: litestream/litestream
    profiles: ["replication"]          # only with --profile replication
    volumes:
      - data:/data
      - ./litestream.yml:/etc/litestream.yml
    environment: [ LITESTREAM_ACCESS_KEY_ID, LITESTREAM_SECRET_ACCESS_KEY ]
    command: replicate

  minio:                               # OPTIONAL convenience only — not shipped as part
    image: minio/minio                 # of the product; a deployer's local S3 target.
    profiles: ["replication-local"]    # Any S3-compatible bucket works just as well.
    command: server /data --console-address ":9001"
    volumes: [ minio:/data ]

volumes: { data: {}, minio: {} }
```

- `docker compose up` → app only, single SQLite file, nothing else. *Easy setup.*
- `docker compose --profile replication up` → adds Litestream streaming the WAL to the
  configured bucket. *Safety.* Make this profile run Litestream as the **supervisor** of
  the app (`litestream replicate -exec`) so a fresh/empty volume auto-restores from the
  last backup before the app starts — backup without auto-restore is only half the safety
  story.
- `--profile replication-local` adds MinIO so the whole thing is self-contained for
  homelab use — purely a convenience for operators without an existing bucket.

This Compose file is a **documented example**, not a shipped product surface. What you
release is the image and its config contract (origins, data volume, optional Litestream
env); operators compose it into their own infrastructure.

Set expectations in docs: Litestream is single-writer **async backup / point-in-time
restore**, not live high availability. If a deployer truly needs hot failover, that's
Turso/libSQL territory and a larger commitment — out of scope for the default product.

## 13. Build-time vs runtime dependencies

- **Runtime (shipped):** the Go binary + embedded assets, SQLite (embedded), a mounted
  data volume. That's the whole product surface.
- **Runtime (operator-supplied, optional):** a TLS-terminating proxy of their choice,
  Litestream + an S3-compatible bucket (or MinIO) for backup, a Chromium thumbnail
  worker. None of these are part of a release — they're things a deployer adds around
  the image.
- **Build-time only:** Go toolchain, Node + esbuild (to bundle CodeMirror and vendor
  the Phosphor icon assets — see `build_assets.md`), `goose` (migrations are embedded
  and run from the binary). Dev-only: golangci-lint (`make lint`, not vendored) and
  ESLint for the editor workspace (§5).

The deliberate outcome: in production it's one small image and one process by default,
with safety and richness added as opt-in Compose profiles — matching the spec's promise
that the easy path and the serious path share almost all of the same system.

The Node-built assets (CodeMirror bundle, Phosphor Icons) are generated into
`internal/api/assets/` at build time and **not** committed to git. See `build_assets.md`
for the workspace layout, the `scripts/build-assets.sh` entrypoint, and how the
Dockerfile's Node stage feeds `go:embed`.
