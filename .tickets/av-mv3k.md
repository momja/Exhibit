---
id: av-mv3k
status: closed
deps: []
links: []
created: 2026-08-20T15:00:00Z
type: feature
priority: 2
assignee: Max Omdal
parent: null
tags: [render, security, backend, frontend]
---
# Camera & microphone: extend the sandbox permissioning framework to capture devices

Downloads (av-ryby), clipboard (av-hll6) and external links (av-r0dk) each get a per-artifact, first-use approval and a host-mediated bridge. Camera and microphone are the fourth and fifth members of that family — same per-artifact, first-use decision, same channel — but a **gate** rather than a bridge, because the capability cannot be re-granted in the frame by anyone.

**Two measured facts shaped this, and they are the reason it is not a fourth bridge.**

1. *The frame cannot reach a device.* `getUserMedia` from the sandbox's opaque origin throws `SecurityError: Invalid security origin` before any permission is consulted. `allow="camera; microphone"` on the iframe does not change it — Chrome refuses it even with `--use-fake-ui-for-media-stream` (auto-accept) set, so the refusal is structural, not a prompt outcome. Same no-op as the clipboard delegation (av-hll6).
2. *The host cannot hand one in.* The download bridge's trick is "acquire on the app origin, transfer the payload in", and there is nothing to transfer: a camera `MediaStreamTrack` is not a transferable object in any shipping engine. `postMessage` with one in the transfer list throws `DataCloneError` — to a cross-origin frame, to a same-origin frame, and to a Worker (all three measured, Chrome 141).

A transfer-based bridge was built first and then removed: its primary path never executed and its fallback was taken 100% of the time. Shipping that would have been a dial nothing turns.

**Why the grant must also govern a top-level render.** A browser permission is granted per *origin* and every artifact shares one render origin, so a visitor who allowed the camera for one artifact opened directly has allowed it for **every** artifact on that origin — no per-artifact decision anywhere. Permissions Policy is per *document*, so the render response carries a per-artifact `Permissions-Policy: camera=…, microphone=…` built from the same two approvals. Verified against a browser that already held the origin permission: unapproved is refused, camera-approved gets video and is still refused audio. This is a deliberate behavior change — an unapproved artifact can no longer reach a device even opened directly.

## Scope

**Schema/store**: `camera_approved`, `microphone_approved` on `artifacts` (migration 027, next unused goose version). Two columns, not one `media_approved` — a dictation tool approved for a microphone was never approved for a camera. `store.ApprovalColumns` names the whole capability-approval set once, so the API's strict-bool check and the store's cannot drift.

**Render surface** (`internal/render/render.go`):
- `buildPermissionsPolicy(camera, microphone)` → `camera=(self)|()`, `microphone=(self)|()`, set in `serveDoc` beside the CSP. Names those two features only; every other feature keeps its browser default. A widget render passes both false whatever the artifact holds (a tile renders unattended behind `pointer-events:none` — no gesture to attribute a prompt to).
- Media **gate** inside `bridgeScript` (framed-only, so absent from the widget preamble by construction): replaces `navigator.mediaDevices.getUserMedia`, reports which devices were asked for, and settles by rejecting with a `DOMException` — never resolving a stream, never hanging. Constraints stay in the frame: the host acquires nothing, so sending them would be data with no reader. Constraints naming neither device reject with the `TypeError` the native call throws. `navigator.mediaDevices` absent entirely gets a minimal object rather than leaving an artifact to die on property access.

**Host** (`web/gallery/detail.js`): `__avMedia` listener validated like every other bridge message (`e.source === frame.contentWindow`, `d.artifactId === ID`). Unapproved → prompt naming exactly the devices asked for; **Allow and open** persists only those and opens the top-level render (the click's activation covers the PATCH roundtrip). Already approved → no second prompt; the frame raises the av-yvtb capability banner instead. Every path replies, including a request displaced by a newer one — a `getUserMedia` left pending is a hang, which is what this gate exists to remove.

**UI**: first-use modal on the detail page; `cam-select`/`mic-select` on the edit page's security panel, dirty-flagged like `link-select` (the bootstrap value goes stale the moment the viewer approves in another tab); `ph-camera`/`ph-microphone` rows and glyphs in the capability cluster and popover; the "Fully contained" copy extended.

**Docs**: security.md §4 (renamed to include capture devices), architecture.md §3.1/§3.2/§6, technical_stack.md §4, PRD §6.3 (new) + §4.4 schema.

## Acceptance Criteria

- `PATCH /api/artifacts/:id {"camera_approved"|"microphone_approved": true|false}` round-trips; a non-bool is a 400; approving one device never approves the other.
- An unapproved artifact's render document carries `camera=(), microphone=()`; approving the camera yields `camera=(self), microphone=()`.
- A share carries the owner's approvals; a widget render carries neither, even when its artifact holds both.
- The detail-page iframe sandbox is unchanged (`allow-scripts allow-forms`) and carries no `allow=` delegation, whatever the approval state.
- The widget preamble contains no `__avMedia` (extends the existing strict-subset assertion); the artifact preamble does, inside the framed guard.
- The frame's `getUserMedia` always settles: denial, displacement, and the already-approved case all reject rather than hang.

## Verified in a browser (Chromium 141, fake devices)

1. Unapproved artifact, camera requested in the preview → prompt naming "your camera".
2. Allow → `camera_approved` true, `microphone_approved` still false, top-level render opened in a new tab.
3. The artifact's promise settled (`NotSupportedError`), no hang.
4. Top-level render → `OK tracks=1 kind=video state=live`. The camera actually works there.
5. Second request in the preview → no re-prompt; capability banner shown; promise settled.
6. Enforcement, with the browser's own permission for the render origin already granted: unapproved → `camera=(), microphone=()` and both `getUserMedia` calls refused; camera-approved → video succeeds, audio still refused.

## Notes / follow-ups

- **Synthesizing a stream in the preview was considered and deliberately not done.** The frame could be fed `ImageBitmap` frames (→ canvas → `captureStream`) and PCM (→ `AudioContext` destination) to produce a `MediaStream` object. That is a picture of a device, not a device: no real constraints, no `applyConstraints`, no `getSettings`, and a `stop()` reaching no hardware. It is a rendering feature and deserves its own ticket and its own trade-off discussion.
- `enumerateDevices` is deliberately not bridged: the host's device list is the visitor's hardware inventory and nothing in the frame needs it. Where `navigator.mediaDevices` had to be synthesized it resolves empty, which is what an unpermitted context reports anyway.
- No CSP interaction. A capture device is local I/O; what an artifact does with the captured bytes is governed by `connect-src` like any other data in the frame.
- security.md §4 gains a fourth preamble family, **capability gate**, for exactly this shape: the sandbox denies it and the host cannot re-grant it, so the preamble records the decision and enforcement lives elsewhere (a response header).
