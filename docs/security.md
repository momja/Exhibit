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
  storage API (`localStorage`/`sessionStorage`; IndexedDB and `window.storage`
  deferred) and swaps its *backing* to the server behind an unchanged surface
  → portable, cross-device state.
- **Capability bridge** — re-grants a capability the sandbox *denied*
  (clipboard, downloads) by proxying the op to the trusted host under
  first-use approval. Not persistence. This section.
- **Polyfill** — reconstructs an API *absent* in this environment (e.g. File
  System Access pickers, deferred as av-70t9) atop available primitives.

The capability-registry work (av-u0vc) covers the **capability-bridge family
only**; storage adapters and polyfills are orthogonal axes it does not touch.
Bare "shim" never means the whole preamble — say "render preamble."

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
