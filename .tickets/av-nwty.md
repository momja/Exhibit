---
id: av-nwty
status: open
deps: []
links: [av-hll6, av-70t9]
created: 2026-07-19T23:48:59Z
type: feature
priority: 2
assignee: Max Omdal
tags: [shim, renderer, security]
---
# Persistent live file access — host-frame FSA handle bridge (single-device)

Live upgrade to av-70t9's <input> polyfill so a file picked from disk stays connected across sessions and reflects later on-disk edits, without re-picking.

Today the polyfill (internal/render/render.go, av-70t9) returns a one-shot File snapshot, so users re-select every time. Snapshotting the bytes as artifact state is explicitly REJECTED: it breaks the "the data is coming from my machine" assumption. Scenario that must work: open a disk file from an artifact; edit that file a week later; reopen the artifact a week after that -> the artifact must see the edits.

Root reason no in-sandbox shim can do this: the sandbox opaque origin can never mint a real FileSystemHandle (Blink SecurityError, documented in av-70t9), and a real handle is the only *live* pointer to the on-disk file. A real handle + cross-session persistence require a stable secure origin with real IndexedDB and re-grantable permissions — which only the host frame (APP_ORIGIN) has. This revives the "host-frame picker bridge" that av-70t9 listed as its Rejected alternative, now justified by persistent live-file access rather than just read-a-file.

## Design

TAXONOMY (two persistence classes, different homes):
- Server-backed, cross-device: storage shims (localStorage / sessionStorage / future IndexedDB). Synthetic artifact state; syncing everywhere is the point.
- Host-local, single-device: real file handles. A pointer to THIS machine's disk. Contents and handle NEVER touch an HTTP endpoint. Single-device is intentional and desired (local control, anti-SaaS), not a limitation.

MECHANISM (host-frame FSA bridge):
1. Sandbox polyfill upgrade (render.go): when a host frame exists AND real FSA is available there, showOpenFilePicker / showDirectoryPicker forward to the host over the id-correlated postMessage bridge (same shape as the clipboard bridge av-hll6). Host runs the REAL picker (transient user activation propagates iframe -> ancestor), obtaining a live handle.
2. Host persists the live FileSystemHandle in the host frame's OWN IndexedDB (browser-local, app origin), keyed by (artifact id, slot). Slot = the FSA {id} option, else one default slot per artifact. The handle never leaves the browser and is never serialized to the server (a structured-clone shipped to a server comes back an inert husk anyway).
3. Handle methods proxied across the frame: getFile() -> host reads CURRENT bytes and posts File/Blob contents back (contents only, never the handle — consistent with av-70t9's rejected-alt note); createWritable()/write/close proxied for write-back; directory iteration proxied.
4. Reconnect is lazy: on first getFile()/access this session the host calls handle.requestPermission() (one gesture-driven click, e.g. "Reconnect to config.json?") — not on load, so an artifact that never touches the file never prompts.
5. Approval gate: filesystem_approved boolean (first-use, revocable in Edit), mirroring downloads_approved / clipboard_approved. This bit is metadata, not file data — the ONLY server-side artifact of the feature. Micro-decision: could be host-local instead for purity; default server-side for Edit-UI consistency.

FALLBACKS (av-70t9 stays the floor, no regression):
- No host frame (top-level render / share page) -> current <input> polyfill.
- Non-Chromium (no persistable FSA) -> current <input> polyfill.
- No stored handle for (this device, artifact, slot) -> pick once, then persist. A second device simply has no handle -> re-pick. Degrades to "pick again," NEVER to stale/wrong data.

EXPLICITLY NOT dependent on the deferred IndexedDB storage shim (docs section 5.2). That shim backs to the server for cross-device sync; live handles must stay in-browser on a stable origin, so it is the wrong mechanism. Different home, no dependency.

CEILINGS (browser-imposed, no design removes them):
- One gesture-driven requestPermission() per session (Chromium requirement; same as VS Code Web / Google Docs "restore access"). If exhibit is ever installed as a PWA on APP_ORIGIN, Chrome persistent-permission grants can make previously-approved handles zero-click — a later enhancement, and another reason this lives on the app origin.
- Chromium-only for the live path.

## Acceptance Criteria

- Open a file via showOpenFilePicker, close/reopen the artifact in a later browser session; after one reconnect click, getFile() returns the file's CURRENT on-disk bytes — edits made externally between sessions are visible. No re-pick.
- File contents and the handle never traverse any HTTP endpoint: verify no PUT/POST carries file bytes; only the filesystem_approved PATCH is sent, and it carries no file data.
- filesystem_approved gates first use, the prompt names the artifact + file, it is revocable in Edit, and denial rejects the picker call with a normal DOMException the artifact handles.
- Handle persisted in the host frame's IndexedDB keyed by (artifact, slot); a second artifact/slot never sees another's handle.
- Fallbacks: no host frame, non-Chromium, and no-stored-handle each fall back to the av-70t9 <input> polyfill with no regression.
- showDirectoryPicker live path re-reads current directory contents on reconnect (Chromium); falls back to webkitdirectory otherwise.


## Notes

**2026-07-20T00:06:24Z**

Defaults confirmed by owner (not assumed): (1) single per-artifact `filesystem_approved` boolean, not per-slot grants; (2) approval bit stored server-side for Edit-UI consistency. The "micro-decision" in Design is settled — server-side.
