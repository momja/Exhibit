---
id: av-ryby
status: open
deps: []
links: [av-hll6]
created: 2026-07-06T22:17:56Z
type: bug 
priority: 2
assignee: Max Omdal
---
# User reported error message: Download is disallowed. The frame initiating or instantiating the download is sandboxed, but the flag ‘allow-downloads’ is not set. See https://www.chromestatus.com/feature/5706745674465280 for more details.

Downloads from artifacts fail because the render iframe's `sandbox="allow-scripts"` deliberately omits `allow-downloads`. Export-a-file is a core capability for tools (CSV generators, image editors), so silent failure is a real defect — but blanket-adding `allow-downloads` lets artifact code trigger downloads programmatically, which we don't want for untrusted/shared artifacts.

## Design: host-mediated download bridge with first-use approval

Decision: do **not** add `allow-downloads`. Instead, mediate downloads through the host frame with an iOS-style ask-on-first-attempt prompt, per artifact. This is feasible without compromising isolation because it reuses the exact architecture the storage shim already proved (architecture §6):

1. **Shim intercepts the common download vectors** inside the iframe: capture-phase click handling for `a[download]` and anchors with `blob:`/`data:` hrefs (covering both user clicks and programmatic `a.click()`). It prevents the default and posts the payload bytes + filename to the host frame via `postMessage` (ArrayBuffer transfer — the bytes cross the boundary as data, not as a capability grant).
2. **Host prompts on first attempt for this artifact.** Approval is persisted server-side next to the network allowlist (e.g. a `downloads_approved` flag PATCHed through the API — single write path). Subsequent downloads from an approved artifact proceed without prompting; the per-artifact settings surface can revoke.
3. **On approval, the host triggers the download itself** from the app origin (construct an anchor from the transferred bytes). This works because transient user activation propagates from the iframe click to ancestor frames.
4. **The sandbox remains the wall.** Vectors the shim doesn't catch (navigation-triggered downloads, exotic APIs, a malicious artifact deleting the shim's hooks) simply stay blocked by the missing `allow-downloads` flag. Same philosophy as ingest scanning: the shim is UX/transparency, the browser sandbox is enforcement. Evading the shim gains nothing — it just makes the download fail.

Shared `/s/:id` pages: no bridge (no authenticated host, and av-7k7b serves shares shim-less), so downloads stay blocked for share visitors in v1.

Same bridge pattern as av-hll6 (clipboard) — if built together, factor a small capability-bridge protocol in the shim/host rather than two ad-hoc message types.

## Acceptance Criteria

- An artifact using `a[download]` with a blob/data URL produces a working download after the user approves the first attempt; the prompt names the artifact and filename.
- Approval persists across reloads/devices (server-side), is revocable from artifact settings, and denial blocks the download without breaking the artifact.
- The iframe sandbox attribute still omits `allow-downloads`; no download occurs for unapproved artifacts by any vector.
- /docs (security.md + architecture) document the bridge and the first-use approval model.


## Notes

**2026-07-12T17:05:10Z**

Download bridge cherry-picked onto current main in PR #41 (feature/av-hll6/capability-bridge), landing together with av-hll6 clipboard on the shared bridge. Original standalone PR #31 was closed unmerged.
