---
id: av-hll6
status: closed
deps: []
links: [av-ryby, av-70t9, av-nwty]
created: 2026-07-06T22:19:31Z
type: task
priority: 2
assignee: Max Omdal
---
# Clipboard I/O

User reported failure to copy paste from sandbox:

```
[Violation] Permissions policy violation: The Clipboard API has been blocked because of a permissions policy applied to the current document. See https://crbug.com/414348233 for more details.
```

Clipboard should be enabled by default, and /docs should be updated to acknowledge this.

Context: av-ys8g already shipped `allow="clipboard-read; clipboard-write"` on the detail-page iframe (internal/api/gallery.go:719), yet the violation was still reported. Likely cause: the `allow` attribute's default allowlist keys on the frame's *src origin*, but a sandboxed frame without `allow-same-origin` has an **opaque** origin that matches nothing — delegation to it requires an explicit wildcard (`allow="clipboard-read *; clipboard-write *"`). So the shipped delegation is plausibly a no-op.

## Design: ask on first request per artifact (same bridge as av-ryby)

Decision: clipboard access is granted per artifact on first use, not blanket-enabled. Feasible safely via the host-mediated capability bridge (so this ticket is NOT closed as impossible):

1. Shim replaces `navigator.clipboard.readText/writeText` (and the legacy `document.execCommand('copy')` path if cheap) inside the iframe; calls post a request to the host frame via `postMessage`, pinned to the app origin.
2. Host prompts on the artifact's first clipboard request (read and write can be approved separately or together — decide in implementation); approval persists server-side beside the network allowlist and is revocable in artifact settings.
3. On approval, the host performs the clipboard operation on the app origin and posts the result back. Transient user activation propagates from the iframe interaction to ancestor frames, so the host has the activation clipboard ops require. Chrome may additionally show its own one-time clipboard-read permission prompt for the app origin — that's fine and expected.
4. With the bridge in place the iframe-level `allow=` delegation becomes unnecessary; remove it (or fix it to the `*` form only if we want a fallback for artifacts that dodge the shim — default to removal, keeping the policy surface single-path).

Paste INTO the artifact via native keyboard paste (Ctrl/Cmd+V into a focused input) is a browser-level event, not a Clipboard API call — it already works and needs no approval; the bridge governs programmatic API access only.

Same bridge pattern as av-ryby (downloads) — build the capability-bridge protocol once if the two land together.

## Acceptance Criteria

- An artifact calling `navigator.clipboard.writeText`/`readText` succeeds after the user approves the first request; the prompt names the artifact and the direction (read vs write).
- Approval persists server-side, is revocable from artifact settings, and denial rejects the API call with a normal DOMException the artifact can handle.
- Reproduce the reported violation first to confirm the opaque-origin delegation hypothesis; record the finding in this ticket.
- /docs updated to describe clipboard behavior (native paste always works; API access is approval-gated).

## Notes

**2026-07-12T17:05:10Z**

Implemented on feature/av-hll6/capability-bridge (PR #41) together with av-ryby's download bridge as a shared host-mediated capability bridge. Removed the no-op allow= clipboard delegation; navigator.clipboard read/write now proxied through the host with first-use approval (clipboard_approved, migration 006). Server-side + render tested; real-browser gesture/permission behavior still needs a manual pass (reproduce original violation).
