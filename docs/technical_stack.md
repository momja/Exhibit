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
| Blob store | Local FS **or** S3-compatible bucket behind the `Blob` interface, selected by config (§3) | — |
| S3 client | `minio/minio-go/v7` | AWS SDK v2 (more modules, AWS-shaped) |
| Source view/edit | **CodeMirror 6** | Monaco (heavier) |
| Artifact renderer | Sandboxed `<iframe>` + per-artifact CSP | — |
| Tier-2 transpile | Babel standalone (in-iframe) → `esbuild` later | SWC |
| Storage shim | Vanilla JS, bundled with `esbuild` | — |
| Ingest scan | `x/net/html` parser (+ JS heuristic) | — |
| Thumbnails | Headless Chromium worker (`chromedp`) — optional | client `html2canvas` |
| Gallery UI | Server-rendered stdlib `html/template` + static CSS/JS assets (§9) | templ (codegen — rejected) |
| Partial re-render | **htmx** — self-hosted / embedded on app origin, no CDN (§9) | hand-rolled fetch-and-swap helper |
| Response compression | `chi` `middleware.Compress` — gzip, explicit type allowlist | brotli (better ratio, new dependency) |
| Agent harness | **Pi** (`pi --mode rpc` sidecar per session; TS tools extension; keys AES-GCM at rest; per-session scoped API credentials; `internal/mockllm` for tests) | Claude Agent SDK (heavier, vendor-tied) |
| Icons | **Phosphor Icons** — self-hosted / embedded on app origin, no CDN (§9) | Lucide / Heroicons |
| Login (optional) | Generic OIDC via discovery — `coreos/go-oidc/v3` + `golang.org/x/oauth2`, exchanged once for our own session (§10) | auth at the operator's proxy (also supported); vendor SDK (rejected) |
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
search with zero extra infrastructure. It indexes artifact title, the visible text
of the artifact source (markup/script/style excluded), and tag names — a single
search box query matches any of the three.

**Migrations: `goose`.** Embed migration files in the binary (`go:embed`) and run them on
startup so a fresh container self-initializes.

**Blob store: two implementations behind the `Blob` interface, chosen by
configuration (av-52ll).** Artifact bodies go either to a mounted volume
(`blob.FSStore`, the default) or to an S3-compatible bucket (`blob.S3Store`).
`BLOB_S3_BUCKET` is the selector and unset is the filesystem, in the
`OIDC_ISSUER` shape: absent means the feature does not exist, and a self-hoster
gains no required configuration. For self-hosters who *do* want object storage,
the MinIO container in the Compose file (§12) is the natural local S3, and is
also what the suite is tested against.

**S3 client: `minio-go`, not the AWS SDK.** The target is S3-*compatible* rather
than AWS — MinIO is the reference — and minio-go is the client shaped for that:
one module, an endpoint as a first-class parameter, path-style addressing
handled for you. AWS SDK v2 would have worked and costs five modules plus
`BaseEndpoint`/`UsePathStyle` ceremony to reach the same place, which is the
wrong trade for a service whose whole pitch is one small image. No AWS-specific
feature is used, so a swap stays a change inside `internal/blob`.

Three properties the implementation must not lose, all pinned by the shared
contract suite that runs against **both** backends: **the backend buffers no
more than one 5 MiB part** in either direction (scope matters — callers above
`Blob` still `io.ReadAll` a body, so this is a promise about the storage layer
and not about the service); **a missing blob fails at `Get`**, forced by a
one-byte read rather than a `Stat` that would cost a second round trip on every
read; and **`Delete` does no existence check**, the interface's idempotent
contract existing precisely because `DeleteObject` already succeeds for a
missing key. `architecture.md` §3.3 carries the reasoning, including why a
partially-read reader is deliberately treated as unknown-length.

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
- Inject a generated **per-artifact CSP** (`connect-src`/`script-src`/`worker-src`/
  `style-src`/`img-src`/`font-src`/`media-src` built from the artifact's allowlist) into
  the served document. The browser enforces the network boundary; this is the wall behind
  §6 of the main spec. Inlined/local sources are exempt from the allowlist since they
  carry no network egress: `style-src` always carries `'unsafe-inline'`,
  `img-src`/`font-src` always carry `data:`, `media-src` always carries `blob:`, and
  `script-src`/`worker-src` always carry `blob:`/`data:` — so inline styles, inlined
  `data:` images/fonts, a locally imported file played back via
  `<video>`/`<audio src=blob:...>` (`URL.createObjectURL` on a picked `File`), and a
  script or Worker the artifact constructs at runtime (ffmpeg.wasm and friends) all
  run without approval. `worker-src` is emitted explicitly rather than inherited from
  `script-src`: when it is missing the worker fails silently, constructing fine but
  never executing its body.
- Inject the **render preamble** — the **storage shim** (§6 here) with the artifact's
  current state inlined, plus the out-of-line **asset manifest** (av-20fk) that
  redirects the page's own `fetch` of a vendored payload to that artifact's asset
  route — into `<head>` *before* any artifact script runs.
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

- Replace `localStorage` and `sessionStorage` with **two** objects implementing the
  `Storage` interface, each over its **own** in-memory cache — one factory, two calls.
  They are separate namespaces in the standard and must stay separate here: a key
  written to one is not readable from the other.
- `localStorage` is the persisted one. Its cache is **inlined into the storage shim by
  the render surface** at request time, so `getItem` is correct on the first
  *synchronous* read (a load-time `fetch` would race the artifact's startup reads and
  lose).
- On `localStorage.setItem`, update the cache synchronously, then **`postMessage` the
  change to the host frame** (pinned to the app origin). The host — same origin as the
  API and authenticated — performs the `PUT /api/artifacts/:id/state`. The storage shim
  itself makes **no network calls**: the sandbox's opaque origin can't reach the API
  cross-origin, so it never has to, and `connect-src` need not include the app origin.
- `sessionStorage` gets the second object with **no persist callback**: memory only,
  no `postMessage`, no stored state. It is installed **only when framed**, alongside the
  capability bridges under the same `window.parent !== window` guard. In the sandbox
  that is exactly native behavior — a sandboxed context is assigned a fresh opaque
  origin per navigation, and storage is keyed by origin — and installing *something* is
  forced, because an opaque origin has no storage key and the native getter throws a
  `SecurityError` on property access. Top-level the document has a real origin where
  native `sessionStorage` is correct and reload-surviving, so it is left in place.
- `IndexedDB` interception and the `window.storage`-style async API are **deferred**
  (build-order step 2 remaining). v1 ships `localStorage` and `sessionStorage` only.
- Last-write-wins on conflicts (`localStorage`; nothing is stored for `sessionStorage`).

Keep this as a single audited file — it's security-sensitive (it sits between untrusted
artifact code and your API) and should be easy to read end to end.

## 7. Ingest scan

Purpose is **transparency, not enforcement** (the CSP is the wall). On ingest, extract
the outbound network footprint to show the user for approval.

- Parse HTML with `golang.org/x/net/html` (a real tokenizer, never regex) to collect
  origins from `src`, `href`, `action`, `<link>`, and ESM import URLs.
- For JS `fetch` targets, accept that full static analysis is impossible — as built,
  `scanner.LiteralRefs` runs two regexes over the raw document for `fetch("…")` and
  `import`/`from "…"` literals, and anything they find is a hint, not analysis. Only a
  literal adjacent to the call is seen: a URL assembled at runtime is missed, as are
  `XMLHttpRequest`, `new Worker`, and `WebSocket` targets. Whatever it misses is caught
  at runtime by the CSP allowlist.
- The snapshot vendorer's runtime-asset pass shares the fetch half of that definition
  (`scanner.FetchRefs`), so the payloads it vendors cannot drift from the fetch targets
  the footprint reports; ESM import refs stay with the scanner only, because the
  module loader never consults `window.fetch` and those origins are governed by
  `script-src` instead. It compensates for the heuristic's blind spot differently:
  rather than rewriting the literals it found, the render surface installs a `fetch`
  wrapper that matches on the resolved URL at call time — so a runtime-constructed URL
  is served when that same absolute URL also appears as a literal fetch ref, and only
  then. Since av-20fk the payloads themselves live outside the artifact body, as blobs
  served from a per-artifact, immutable, cacheable route; the body keeps the literals
  it was ingested with, so nothing an agent does to the document can break the
  redirect.

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
server-rendered grid), tag/collection filters, a detail view, and an add-artifact
page at `/new` (av-qo0j) — and full-page server
renders cover it, keeping everything inside the one Go binary with no frontend
framework and no template codegen. (templ — the codegen engine an early scaffold
used — was considered for the extraction and rejected: the stdlib engine adds zero
dependencies and no generate step, and its contextual auto-escaping replaces the
hand-rolled HTML escaping the old string-concatenated pages needed.)

CodeMirror and the renderer iframe are islands of client JS inside these
server-rendered pages.

**Partial re-render: htmx (av-6m3e).** When server-side state changes after
load, the page re-fetches one server-rendered fragment and swaps it in rather
than reloading (a reload drops live iframes, editor buffers, and SSE streams)
or rebuilding the markup in JS (a second definition of the same component, in a
second language). htmx buys the attribute vocabulary that keeps that wiring in
the markup — trigger, target, swap — for ~15 KB and no build step, which is why
it won over a hand-rolled fetch-and-swap helper. The rules it must follow:

- Fragments render the **same named `html/template` partial** as the full page,
  so a component has exactly one definition. The fragment routes live under
  `/partials/*` next to the page routes.
- htmx is **self-hosted on the app origin, never a CDN** — vendored at build
  time by the `web/htmx/` workspace into the embedded assets, exactly like the
  Phosphor icons below, and loaded from `/assets/htmx/htmx.min.js`.
- Page JS keeps no cached references into a swappable region: after a swap the
  old nodes are gone. Resolve elements on use.

Shipped consumers: the agent surface's preview pane, re-rendered after every
agent save (`architecture.md` §3.7, `docs/agent.md`); and the artifact edit
page's widget panel, which swaps `/partials/card-widget` after a save so the
tile refreshes without a reload that would drop the CodeMirror buffer beside it
(`docs/widgets.md`).

**Home-screen app shell (av-fdcx).** Every app-origin page head includes the shared
`pwaHead` partial: the `manifest.json` link plus the `apple-*` tags iOS reads instead
of the manifest's display mode. It is markup only — no script, and nothing that
touches the viewport meta. None of it reaches the render origin: an artifact is a
visitor-authored file and sets its own viewport, or doesn't.

**Form fields are 16px on touch (av-3qmf).** iOS Safari zooms the page in whenever it
focuses a field whose text is under 16px, and does not zoom back out on blur — the
page is left wider than the screen, with the submit button beside the field pushed
off it. The whole fix is in the type scale: `tokens.css` defines
`--field-font-size` / `--field-code-font-size` (14px / 12px), a single
`@media (pointer:coarse)` block raises both to 16px, and every input, select,
textarea, and CodeMirror instance sizes itself from those tokens — plus an
element-level floor in `components.css` for controls no rule names, since an unstyled
input inherits the UA's ~13px. 16px is the exact threshold WebKit uses, so removing
the *reason* for the zoom leaves zooming itself fully available: no
`user-scalable=no`, no gesture handlers, no WCAG 1.4.4 exposure. The query is on
pointer type rather than width, because a narrow desktop window has no on-screen
keyboard and a landscape tablet is wide and still touched. The standing rule when
adding a control: size it from the token, never a literal `px` — a hardcoded size
opts it out of the bump silently, and the symptom only shows up on a phone.

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

Two credentials, checked in that order by one `chi` middleware:

- **A session cookie**, when the deployment has a login (below). Browser requests
  carry it automatically, and the session is looked up per request.
- **A static bearer token** (`AUTH_TOKEN`) otherwise — the API/CLI credential, and
  the only credential a single-user instance has. With no login configured this is
  exactly the check it has always been, with `owner_id` fixed at `1`.

A server-rendered page embeds whichever of these the *request* earned, never the
process's token unconditionally: with a session, none — the cookie authenticates
the page's own fetches, and an embedded token would survive the logout that
deletes the session; with no identity provider, the static token as before. See
`security.md` §1.5.

**Login is optional (av-30rj, av-q30x, av-rzvf).** Three supported ways to put one
in front of an instance, and none is more official than the others:

- **At the operator's reverse proxy.** Authelia, Tailscale, oauth2-proxy, or plain
  basic auth gate the request before it reaches the app. Nothing is configured in
  Exhibit — consistent with TLS and proxying already being the operator's (§12).
  Gate the *app* origin; the render origin serves shares to people with no account.
- **Exhibit's own accounts** — the path that needs nothing else running, and the
  reason a self-hoster no longer has to stand up an identity server to close
  their instance or to give a second person a library.
  `golang.org/x/crypto/bcrypt` is the only dependency it adds. Accounts are rows
  in `users` with a nullable `password_hash`, provisioned by the operator
  (`server user add` / `user passwd`) rather than by self-registration, so there
  is nothing to verify and no reset mail — and therefore no SMTP in the config
  surface. `LOGIN_USERNAME` / `LOGIN_PASSWORD_HASH` remain as the bootstrap and
  break-glass credential. Either way the operator supplies a bcrypt hash rather
  than a password the service hashes for itself, because hashing a plaintext the
  environment already holds beside it protects nothing.
- **An OIDC provider**, via three env vars (`OIDC_ISSUER`, `OIDC_CLIENT_ID`,
  `OIDC_CLIENT_SECRET`). Authorization Code + PKCE, with every endpoint and signing
  key discovered from the issuer's `/.well-known/openid-configuration` — discovery
  is what makes "any provider" configuration rather than code. Libraries are
  `coreos/go-oidc/v3` + `golang.org/x/oauth2`, the conventional Go pairing and both
  generic; **no vendor SDK is in `go.mod`**, and a different provider is a
  constructor implementing `auth.IdentityProvider`'s two methods.

The provider is exchanged **exactly once**, at `/auth/callback`, for a session of
our own: an opaque random id in an `HttpOnly`, `SameSite=Lax`, app-origin-only
cookie, looked up per request against a `sessions` row. Per-request verification of
a provider-signed token is the API-token pattern and the wrong default here — it
puts a network check in the request path and makes logout impossible, since a
signed token outlives any decision to revoke it. `/auth/logout` deletes the row, so
the credential dies on the next request. Full rationale and the cookie's origin
constraint: `architecture.md` §3.8.

The local credential ends at the same session, through the same call — it is a
second login *path*, not a second session mechanism, and deliberately not an
`IdentityProvider` implementation (that interface is redirect-based; a form post
has no authority to redirect to and no code to exchange). What keeps it small
enough to justify owning passwords at all is that accounts are
operator-provisioned: no self-registration, so nothing to verify; no
self-service reset, so no reset mail and therefore no SMTP; and an admin who
can always reset a forgotten password, so nobody is locked out. bcrypt's cost is
also the rate limiting.

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
  the Phosphor icon and htmx assets — see `build_assets.md`), `goose` (migrations are embedded
  and run from the binary). Dev-only: golangci-lint (`make lint`, not vendored) and
  ESLint for the editor workspace (§5).

The deliberate outcome: in production it's one small image and one process by default,
with safety and richness added as opt-in Compose profiles — matching the spec's promise
that the easy path and the serious path share almost all of the same system.

The Node-built assets (CodeMirror bundle, Phosphor Icons, htmx) are generated into
`internal/api/assets/` at build time and **not** committed to git. See `build_assets.md`
for the workspace layout, the `scripts/build-assets.sh` entrypoint, and how the
Dockerfile's Node stage feeds `go:embed`.
