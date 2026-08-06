# Exhibit — Security

Companion to `architecture.md` (§4 trust boundaries, §6 render flow) and
`product_requirement_doc.md` (§6 security model). Those documents place the
boundaries; this one states the operative stance — what is enforced, by which
mechanism, and which defaults were chosen deliberately.

The one-paragraph threat model: an artifact is **untrusted code that executes in
the visitor's browser**. The server never executes it — artifact bytes are inert
data at rest, stored and served. What must be protected is therefore (a) the app's
session and API from artifact code, (b) the visitor from silent network egress,
and (c) the server itself during ingest-time fetching. Each gets its own mechanism
below, and every hard boundary is browser- or kernel-enforced machinery, never our
own code convention.

## 1. Isolation: two origins and an opaque sandbox

- Artifacts are served only from `RENDER_ORIGIN`, never the app origin. The
  gallery embeds them as
  `<iframe src="RENDER_ORIGIN/a/:id" sandbox="allow-scripts" ...>` —
  **without `allow-same-origin`**, so the frame runs in an opaque (`null`) origin.
  Artifact code cannot read app cookies, real-origin storage, or make
  authenticated same-origin requests; two artifacts cannot read each other.
- The render surface is read-only. It looks up, wraps, and serves — it mutates
  nothing, which is what makes the same path safe to expose unauthenticated for
  share links (`/s/:shareID`).
- Every rendered document carries `frame-ancestors <APP_ORIGIN>` in its CSP, so
  only the app's own pages may embed an artifact, and `Cache-Control: no-store`,
  so a stale document (old render preamble, old state, old CSP) is never
  served from a cache.
- The render preamble's write path is the only channel out of the sandbox: a
  `postMessage` with `targetOrigin` pinned to the app origin. The host page
  accepts a state message only after checking its shape **and** that
  `e.source` is the artifact iframe's own window (the sandboxed frame's
  `e.origin` is `"null"`, so identity is established by source window, not origin
  string). Only then does the host — same-origin with the API and authenticated —
  perform the `PUT /api/artifacts/:id/state`. The artifact itself never holds a
  credential and never reaches the API.

### 1.1 Module workers: an accepted opaque-origin limitation

The opaque origin has one benign casualty. Chrome refuses to fetch a **module**
worker's script for an opaque origin, so a `Worker(url, { type: 'module' })`
constructed inside the sandbox fires `onerror` with an empty message and never
runs — with **no** `securitypolicyviolation`, so it is not a CSP fault and cannot
be relaxed with CSP. Classic `blob:`/`data:` workers run fine in the same frame
(av-x01o); only module workers trip this. The same module worker runs fine when
the artifact is opened **top-level** at `RENDER_ORIGIN/a/:id`, which has a real
origin. Practical impact: ffmpeg.wasm 0.12 always spawns its class worker as
`{ type: 'module' }`, so it transcodes correctly in a new tab or share link but
hangs on "Loading…" in the gallery's embedded preview.

**Stance (av-yvtb): keep the opaque sandbox, detect and warn.** We deliberately
do **not** fix this by giving the frame a real origin (per-artifact subdomains +
`allow-same-origin`). The opaque origin does double duty — it is the trust
boundary *and* the enforcement of "all state is server state": in a
no-`allow-same-origin` frame the real `localStorage` throws, so the storage shim
is the only possible store and cross-device is airtight. A real origin would hand
the artifact a disk-backed store for any surface the shim doesn't cover (e.g.
IndexedDB, still deferred), landing state per-device again. So a real origin
stays an explicit **hardened opt-in**, never the default (spec §12).

Instead the render preamble wraps the `Worker` constructor (framed-only, under
the same `window.parent !== window` guard as the other bridges): when it sees
`{ type: 'module' }` while `self.origin === 'null'` (the effective, opaque
origin — `location.origin` still reports the URL's tuple origin here, so it is
the wrong signal), it `postMessage`s a diagnostic to the host frame (pinned to
the app origin, first occurrence only), then constructs the real worker
unchanged — runtime behavior is not altered; the worker fails on its own as
before. The diagnostic is deliberately **capability-agnostic**: a generic
`__avCapabilityWarning` message naming the capability (`module-worker` in phase 1)
plus an optional resource string, so future detections reuse the same channel and
banner rather than adding message types. The gallery detail page listens for it
and reveals a non-blocking banner: a generic, reusable headline for a
non-technical audience ("This artifact uses unsupported browser capabilities.
Open it directly to run it.") over a default-collapsed `<details>` whose copy is
selected from the reported capability — the specific failure and, when known, its
resource (the worker script URL) — with a generic fallback for any
not-yet-described capability. It offers "Open in new tab" (the top-level render,
which runs it). This converts a silent, indefinite hang into an explained,
actionable state.
`SharedWorker` and service-worker registration fail on an opaque origin too and
are a possible follow-on; phase 1 covers module `Worker`s only, and a new
detection needs only a capability slug plus a copy entry, not a new message or
banner. An agent-assisted rewrite to a sandbox-compatible worker is tracked as
phase 2 of av-yvtb.

### 1.2 Web Storage in an opaque origin

Storage is keyed by origin, and an opaque origin cannot produce a key — so the
sandboxed frame gets no storage area at all. `localStorage`, `sessionStorage`,
and `indexedDB` each throw a `SecurityError` on **property access**, before any
method call, which kills an artifact that reads storage at the top of its
script. The render preamble must therefore install *something* under both Web
Storage names; the only question is what backs each one.

- **`localStorage` → the server.** State is inlined at render and writes bridge
  through the host frame (§1). This is the cross-device store, and the opaque
  origin is what makes it airtight: there is no real per-device store to fall
  back to.
- **`sessionStorage` → a separate, purely in-memory namespace.** Its own cache,
  no write-through, no `artifact_state` rows, nothing leaving the frame. The two
  namespaces are distinct objects over distinct caches, so a key written to one
  is not readable from the other — what the standard requires and what artifacts
  are written against. Because it produces no rows, giving state a principal
  (av-q0ub) left `sessionStorage` untouched: there is nothing stored to scope,
  and a frame-local, per-navigation namespace already belongs to exactly one
  viewer on exactly one device.

In-memory is not a degradation of `sessionStorage` here, it is its native
behavior: a sandboxed browsing context is assigned a **fresh opaque origin on
every navigation**, so native `sessionStorage` would also start empty after each
(re)load, and each frame's origin is unique, so two frames sharing one would be
the wrong behavior. Keeping it out of the server is also the conservative
choice: `sessionStorage` is where artifacts put what should *not* survive, so
persisting it would both invert the lifetime the author chose and turn
throwaway values into durable, cross-device rows.

The `sessionStorage` replacement is **framed-only**, under the same
`window.parent !== window` guard as the capability bridges. Opened top-level at
`RENDER_ORIGIN/a/:id` the document has a real origin where native
`sessionStorage` works, is tab-scoped, and survives a reload — replacing it
there would be a strict downgrade. `localStorage` installs unconditionally,
since it also serves the inlined reads top-level.

`IndexedDB` is not intercepted (deferred). Note it does not quietly fall back
to per-device storage in the frame either — like the others, it throws.

### 1.3 The render origin is sessionless: signed render tokens

`RENDER_ORIGIN` holds **no session and sets no cookie** — asserted by a test
over every route it answers, because this is the failure mode that would break
silently rather than loudly.

The reason is that a top-level `GET RENDER_ORIGIN/a/:id` is not sandboxed. It is
a real-origin document with the artifact's own script inlined into it, so
anything scoped to that origin is readable by the artifact — which can post it
to any origin on its allowlist. A session cookie there would be handed to
untrusted code on every render. So the render origin cannot learn who it is
serving the way the app origin does; and it does need to know, because `/a/:id`
and `/w/:id` were previously unauthenticated, leaving an unguessable id as the
only thing between one owner's artifact — and the state inlined into it — and
anyone who learned that id.

**The credential is a signed URL token instead** (`internal/rendertoken`,
av-c5aq):

- **Scope: one artifact, one owner, ten minutes.** Nothing wider is ever minted
  — no owner-wide token, no collection token, no long-lived one. The narrow
  scope is what makes a URL-borne credential acceptable: the artifact *can* read
  its own token out of `location.href`, and that gains it only the access it
  already has, to itself, for a few more minutes.
- **Shape: HMAC-SHA256 over `(version, artifact id, owner, expiry)`**, encoded
  `<owner>.<expiry>.<tag>` in a `t` query parameter. Not a JWT: one issuer, one
  verifier, one algorithm, so an algorithm-negotiation surface would be pure
  cost. The artifact id is *mixed into the MAC* rather than carried as a field,
  so a token minted for artifact A does not verify on artifact B's route — the
  scoping is the signature itself, not a comparison a verifier could omit.
- **Key: derived from the existing server secret** (`EXHIBIT_SECRET`, or the
  generated `secret.key`), domain-separated from the AES-GCM key that seals
  agent provider keys. One secret for an operator to manage, not two. With no
  secret configured at all the process signs with an ephemeral random key, so
  tokens work but do not survive a restart — the strict answer, since the
  permissive one is an open render origin.
- **Verification is stateless** — no table, no round trip — and **fails
  closed**: no signer, no token, a bad signature, an expired token, or an
  artifact belonging to another owner all answer `404`, identically, so the
  surface is not an existence oracle for other tenants' libraries.

Where a token is minted matters for both cost and staleness:

- **Frames** — gallery card tiles, the detail page's viewer, the edit page's
  widget panel, the agent preview pane, and the `/partials/*` fragments — get
  their token minted **during the page render**, in memory, with the key already
  loaded. A gallery of forty cards costs forty HMACs and no extra I/O.
- **Links** ("Open in new tab") carry **no token at all**. They point at the app
  origin's `/artifacts/:id/open`, which mints and redirects at click time. A
  link sits in an open tab indefinitely, so a token baked into the markup would
  be expired by the time anyone used it — and "copy link address" would spread a
  credential.

The verified owner is also the render surface's **state principal**: the answer
to "whose state should be inlined into this document". That answer is
load-bearing — `artifact_state` is keyed by `(artifact_id, user_id, key)`
(av-q0ub), and the token's principal *is* that `user_id`. A principal with rows
of their own gets exactly those; a principal with none gets an empty cache,
never somebody else's.

`/s/:shareID` is **unaffected and takes no token**: the share row *is* the
authorization (`architecture.md` §7), which is what lets a shared link work for
someone with no account. A share render inlines the artifact owner's state,
because publishing an artifact is publishing it as its owner sees it.

## 2. CSP: the allowlist is the wall

Each artifact carries a set of per-origin decisions (`artifact_network_origins`,
one row per origin). The origins decided `allow` are the allowlist; origins decided
`block` are "don't ask again" markers for the runtime prompt and are never part of
it. At render time the surface generates the document's `Content-Security-Policy`
from the allowlist:

```
default-src 'none'
script-src  'unsafe-inline' 'unsafe-eval' blob: data: <allowlisted origins>
worker-src  blob: data: <allowlisted origins>
style-src   'unsafe-inline' <allowlisted origins>
img-src     data: <allowlisted origins>
font-src    data: <allowlisted origins>
media-src   blob: <allowlisted origins>
connect-src <allowlisted origins, or 'none' if the list is empty>
form-action 'self' <allowlisted origins>
frame-ancestors <APP_ORIGIN>
```

Every source above belongs to one of two buckets, and sorting a new one into the
right bucket is the whole design rule:

| Bucket | Examples | Gating |
|--------|----------|--------|
| Network-reaching | a remote origin fetched, imported, styled from, or submitted to | scan → approve → allowlist (spec §6.2) |
| Local / no-egress | `'unsafe-inline'`, `'unsafe-eval'`, `data:`, `blob:` | unconditional — always present |

A local source runs or renders bytes the artifact already carries, or a file the
visitor picked on their own machine. Nothing leaves the browser, so gating it
behind per-artifact approval buys no security while breaking canonical
single-file patterns.

Points of stance embedded in that policy:

- **`'unsafe-inline'`/`'unsafe-eval'` in `script-src` is deliberate.** The
  artifact *is* an inline script; blocking inline execution would block the
  product. CSP is not doing XSS duty here — containment of what the script can
  *touch* comes from the sandbox and origin isolation (§1); CSP's job is
  controlling what the script can *reach over the network*.
- **Inlined and locally constructed sources are exempt from approval** because
  they are not network requests: `style-src` always permits inline styles,
  `img-src`/`font-src` always permit `data:` URIs, `media-src` always permits
  `blob:`, and `script-src`/`worker-src` always permit `blob:`/`data:`. An
  artifact that carries its own CSS, images, and fonts, plays back a file the
  visitor picked, and spins up a Worker from a `blob:` URL (ffmpeg.wasm and
  friends) renders with zero egress — the "it's just a file" thesis in policy
  form.
- **`worker-src` is emitted explicitly**, not left to fall back to `script-src`,
  because a missing `worker-src` fails *silently*: the `Worker` constructor
  succeeds, no error is logged, no promise rejects, and the worker body simply
  never runs — an indefinite "Loading…" with nothing to debug (av-x01o).
- **A no-network artifact gets `connect-src 'none'`.** Nothing is reachable by
  default.
- **The ingest scan is transparency, not enforcement.** It parses the document
  with a real HTML tokenizer and surfaces the origins the artifact references,
  but its output **never seeds the allowlist** — only origins the user explicitly
  approves are written. A runtime attempt to reach anything else is blocked by
  the browser; the user can approve the origin afterward in the artifact's
  allowlist editor, which updates the CSP on next render.

## 3. Vendoring: snapshot on import, never live-linked

URL ingest fetches the page **once** and stores its body as the artifact.
**Vendoring (inlining) of relative external assets** (images, scripts,
stylesheets, fonts) is tracked by the open `exhibit-lwb` epic
(`exhibit-lwb.3`–`exhibit-lwb.6`); today the top-level document body is stored
verbatim without inlining, so relative asset references still resolve against
the source site.

**Bounded fetcher status:** `internal/snapshot` contains a completed bounded
`Fetcher` component (`exhibit-lwb.2` closed) with per-asset and total size
budgets, an asset-count cap, request timeouts, a redirect limit, and a
**dial-time guard rejecting non-public addresses** (loopback, private ranges,
link-local) to prevent SSRF. However, this bounded fetcher is **not yet wired
into the ingest or refetch paths** (`exhibit-lwb.6` open). Until that wiring
lands, `POST /api/artifacts` (URL branch) and `POST .../refetch` use a bare
`http.Get` with a 10 MiB body cap and no SSRF guard. The bounded pipeline
described here is the target state.

After ingest the stored copy never phones home. Updating it is an explicit user
action (`POST /api/artifacts/:id/refetch`), which re-runs the same bounded
pipeline. There are no live-linked imports and no automatic refresh.

## 4. Local I/O defaults: clipboard and files

**Render preamble taxonomy** (canonical vocabulary for all docs). The JS
injected into the rendered frame as the first `<head>` script(s) — replacing
browser globals before any artifact code runs — is the **render preamble**.
Its pieces share a *delivery mechanism*, not a *purpose*, and by purpose they
are three families:

- **Storage adapter** (established name: *storage shim*) — intercepts a
  storage API (IndexedDB and `window.storage` deferred) and replaces its
  *backing* behind an unchanged surface. `localStorage` is backed by the server
  → portable, cross-device state. `sessionStorage` is a **separate namespace
  over a separate, purely in-memory cache**, never persisted and never sent
  anywhere — see §1.2.
- **Capability bridge** — re-grants a capability the sandbox *denied*
  (clipboard, downloads) by proxying the op to the trusted host under
  first-use approval. Not persistence. This section.
- **Polyfill** — reconstructs an API *absent* in this environment (e.g. File
  System Access pickers, deferred as av-70t9) atop available primitives.

The capability-registry work (av-u0vc) covers the **capability-bridge family
only**; storage adapters and polyfills are orthogonal axes it does not touch.
Bare "shim" never means the whole preamble — say "render preamble."

**Widget renders take a narrowed preamble** (av-fafu). A gallery card's widget
is served from the same render surface under the *artifact's* CSP, so its
network reach is identical — but it gets the storage adapter with writes
short-circuited, and **no capability bridges and no polyfills at all** (not
injected, rather than injected and disabled). The reasoning is the dividing line
this section already draws: a capability bridge re-grants something behind a
*user gesture and a first-use decision*, and a tile renders unattended in a card
behind `pointer-events: none`, where there is no gesture to attribute a prompt
to — and where the artifact's own approvals were granted for the tool the user
opened, not for its tile. A widget's authority is therefore a strict subset of
its artifact's, by construction. See `widgets.md`.

The dividing line for local capabilities: **local interaction with a user gesture
is allowed; anything that produces egress or bypasses a user decision is not.**

- **Clipboard** — `navigator.clipboard` read/write is **mediated by the host
  frame with first-use approval** — a capability bridge on the same
  host-mediation mechanism as downloads (below). An earlier attempt delegated `allow="clipboard-read; clipboard-write"`
  into the frame, but a Permissions-Policy `allow=` keys on the frame's *src
  origin*, which is opaque (no `allow-same-origin`) and matches nothing — so the
  delegation was a no-op and copy/paste still threw a permissions-policy
  violation. The delegation is removed; instead:
  - The clipboard bridge replaces `navigator.clipboard.readText`/`writeText`
    inside the frame and posts each call to the host (pinned to the app origin), correlated
    by request id so the returned Promise settles with the host's answer.
  - On the artifact's **first** clipboard request the host prompts, naming the
    artifact and the direction (read vs write). Approval persists server-side
    (`clipboard_approved`, PATCHed through the API — the single write path),
    survives reloads and devices, and is revocable from the toolbar. Denial
    rejects the call with a `NotAllowedError` `DOMException` — exactly what a
    real blocked clipboard call throws, so the artifact handles it unchanged.
  - Once approved the host performs the op on the app origin (which holds
    clipboard permission and, from the Allow click, transient user activation)
    and posts the result back into the frame.
  - **Native keyboard paste** (Ctrl/Cmd+V into a focused field) is a browser
    event, not a Clipboard API call, so it always works and needs no approval;
    the bridge governs only programmatic API access.
- **File reads** — `<input type="file">` and drag-in work normally: the user
  picks the file, the artifact reads only what was picked, and the contents are
  subject to the same egress rules as any other data in the frame.
- **Downloads** — the sandbox omits `allow-downloads`, so nothing in an embedded
  artifact frame can initiate a download directly. Because export-a-file is a
  core capability for tools (CSV generators, image editors), downloads are
  instead **mediated by the host frame with first-use approval**, reusing the
  render preamble's postMessage channel (§1):
  - The download bridge intercepts the common export vectors inside the frame — anchor
    activations with `blob:`/`data:` hrefs, both user clicks (capture phase) and
    programmatic `click()` — and posts filename + bytes to the host, pinned to
    the app origin. Bytes cross the boundary as transferred data, not a
    capability grant. `blob:` payloads are recovered from a `createObjectURL`
    registry the bridge keeps, so it needs no fetch (`connect-src` is untouched).
  - On the artifact's **first** download attempt the host prompts, naming the
    artifact and the filename. Approval is persisted server-side
    (`downloads_approved`, PATCHed through the API — the single write path), so
    it survives reloads and devices, and is revocable at any time from the
    artifact's toolbar. Denial drops the bytes without breaking the artifact.
  - Once approved, the host reconstructs the file and triggers the download
    from the app origin.
  - **The sandbox remains the wall.** Approval never adds `allow-downloads`;
    vectors the bridge doesn't catch (navigation-triggered downloads, an artifact
    deleting the bridge's hooks) simply stay blocked by the browser. Like the
    ingest scan, the bridge is UX, not enforcement — evading it gains nothing.
  - The bridge only installs when a host frame exists. An artifact opened
    directly on the render origin ("Open in new tab") is a top-level page, not
    a sandboxed frame, so downloads work there natively — the user has
    explicitly navigated to the tool, and the per-artifact CSP still applies
    via the response header. Share pages get no bridge: opened top-level they
    behave the same way; there is no authenticated host to mediate for them.

## 5. Residual risk

Accepted, with eyes open (see the PRD §6.3): the model controls what an artifact
*reaches*, not what it *displays* — a malicious artifact can still render
convincing fake UI. The isolation in §1 caps the blast radius (no real session to
steal). Auth today is a single static bearer token scoped for single-user,
trusted-circle deployment; the middleware seam exists to swap in real identity
without changing the API contract.
