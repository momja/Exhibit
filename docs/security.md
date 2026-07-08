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
  so a stale document (old shim, old state, old CSP) is never served from a cache.
- The storage shim's write path is the only channel out of the sandbox: a
  `postMessage` with `targetOrigin` pinned to the app origin. The host page
  accepts a state message only after checking its shape **and** that
  `e.source` is the artifact iframe's own window (the sandboxed frame's
  `e.origin` is `"null"`, so identity is established by source window, not origin
  string). Only then does the host — same-origin with the API and authenticated —
  perform the `PUT /api/artifacts/:id/state`. The artifact itself never holds a
  credential and never reaches the API.

## 2. CSP: the allowlist is the wall

Each artifact row carries a `network_allowlist` (JSON array of origins). At render
time the surface generates the document's `Content-Security-Policy` from it:

```
default-src 'none'
script-src  'unsafe-inline' 'unsafe-eval' <allowlisted origins>
style-src   'unsafe-inline' <allowlisted origins>
img-src     data: <allowlisted origins>
font-src    data: <allowlisted origins>
connect-src <allowlisted origins, or 'none' if the list is empty>
frame-ancestors <APP_ORIGIN>
```

Points of stance embedded in that policy:

- **`'unsafe-inline'`/`'unsafe-eval'` in `script-src` is deliberate.** The
  artifact *is* an inline script; blocking inline execution would block the
  product. CSP is not doing XSS duty here — containment of what the script can
  *touch* comes from the sandbox and origin isolation (§1); CSP's job is
  controlling what the script can *reach over the network*.
- **Inlined assets are exempt from approval** because they are not network
  requests: `style-src` always permits inline styles, `img-src`/`font-src` always
  permit `data:` URIs. An artifact that carries its own CSS, images, and fonts
  renders with zero egress — the "it's just a file" thesis in policy form.
- **A no-network artifact gets `connect-src 'none'`.** Nothing is reachable by
  default.
- **The ingest scan is transparency, not enforcement.** It parses the document
  with a real HTML tokenizer and surfaces the origins the artifact references,
  but its output **never seeds the allowlist** — only origins the user explicitly
  approves are written. A runtime attempt to reach anything else is blocked by
  the browser; the user can approve the origin afterward in the artifact's
  allowlist editor, which updates the CSP on next render.

## 3. Vendoring: snapshot on import, never live-linked

URL ingest fetches the page **once** and vendors its external assets into the
stored document, so the artifact stays a self-contained file. Everything that
pipeline fetches goes through one bounded fetcher (`internal/snapshot`), which
owns the entire fetch policy:

- per-asset and total size budgets, and an asset-count cap;
- request timeouts and a redirect limit;
- a **dial-time guard rejecting non-public addresses** — loopback, private
  ranges, link-local — so a crafted page cannot use ingest as an SSRF proxy into
  the server's network.

After ingest the stored copy never phones home. Updating it is an explicit user
action (`POST /api/artifacts/:id/refetch`), which re-runs the same bounded
pipeline. There are no live-linked imports and no automatic refresh.

## 4. Local I/O defaults: clipboard and files

The dividing line for local capabilities: **local interaction with a user gesture
is allowed; anything that produces egress or bypasses a user decision is not.**

- **Clipboard** — the gallery's embed delegates
  `allow="clipboard-read; clipboard-write"` into the sandboxed frame. Without
  this, the opaque origin has clipboard access denied outright and copy/paste — a
  common artifact interaction — throws a permissions-policy violation. This is a
  local capability, not network egress: the browser still gates clipboard access
  on a user gesture, and anything an artifact reads from the clipboard can only
  leave the frame via `connect-src`, which the allowlist governs.
- **File reads** — `<input type="file">` and drag-in work normally: the user
  picks the file, the artifact reads only what was picked, and the contents are
  subject to the same egress rules as any other data in the frame.
- **Downloads** — the sandbox omits `allow-downloads`, so nothing in an embedded
  artifact frame can initiate a download directly. Because export-a-file is a
  core capability for tools (CSV generators, image editors), downloads are
  instead **mediated by the host frame with first-use approval**, reusing the
  storage shim's postMessage channel (§1):
  - The shim intercepts the common export vectors inside the frame — anchor
    activations with `blob:`/`data:` hrefs, both user clicks (capture phase) and
    programmatic `click()` — and posts filename + bytes to the host, pinned to
    the app origin. Bytes cross the boundary as transferred data, not a
    capability grant. `blob:` payloads are recovered from a `createObjectURL`
    registry the shim keeps, so it needs no fetch (`connect-src` is untouched).
  - On the artifact's **first** download attempt the host prompts, naming the
    artifact and the filename. Approval is persisted server-side
    (`downloads_approved`, PATCHed through the API — the single write path), so
    it survives reloads and devices, and is revocable at any time from the
    artifact's toolbar. Denial drops the bytes without breaking the artifact.
  - Once approved, the host reconstructs the file and triggers the download
    from the app origin.
  - **The sandbox remains the wall.** Approval never adds `allow-downloads`;
    vectors the shim doesn't catch (navigation-triggered downloads, an artifact
    deleting the shim's hooks) simply stay blocked by the browser. Like the
    ingest scan, the shim is UX, not enforcement — evading it gains nothing.
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
