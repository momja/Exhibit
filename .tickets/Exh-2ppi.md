---
id: Exh-2ppi
status: in_progress
deps: []
links: []
created: 2026-10-19T01:09:20Z
type: feature
priority: 2
assignee: Max Omdal
tags: [security, ui, render]
---
# Confirm direct artifact opens with a per-artifact remembered choice

Opening an artifact with “Open in new tab” changes it from Exhibit's security-restricted embedded view into a top-level render document, but the current action provides no confirmation. Add a confirmation in trusted app-origin UI, using the existing modal patterns. The embedded artifact is a full working view, not a preview; avoid introducing that description outside the exact copy explicitly supplied below.

Use this exact modal title and body supplied by the user, without appending a list of network/device restrictions:

Open outside the preview sandbox?

This tool will run as a regular web page. It can download files, navigate to external sites, and use browser features restricted in the preview, subject to your browser’s normal controls.

Offer Cancel and Open directly, plus an unchecked “Do not ask again for this artifact” checkbox. Only confirming Open directly with that checkbox selected remembers the choice. Subsequent direct opens for that artifact go immediately to a new tab. This is a viewer preference scoped per artifact, not a library-wide decision or a shared permission grant. Persist it across page reloads; isolate it between viewers sharing a browser. If preference storage is unavailable, keep confirmation functional and default to asking again.

Cover every direct-open action from the artifact viewer: toolbar, unsupported-capability banner, and camera/microphone/location flows that open the artifact top-level. Reuse one gate. Keep external destination links and iframe reload/state-sync paths separate from this direct-open confirmation. Do not change the render security policy or the artifact's permissions. Read-only recipients must be able to confirm and remember their own choice without obtaining artifact mutation rights.


## Acceptance Criteria

1. The first direct-open action for an artifact shows an accessible modal in Exhibit UI, with the exact supplied title and body and existing modal styling/behavior.
2. Cancel, Escape, and backdrop dismissal open no tab and remember no choice. Keyboard focus follows existing modal patterns and returns sensibly on dismissal.
3. Open directly opens the current artifact in one new tab using the fresh-token app-origin /open route and noopener. Without the checkbox the next direct-open action asks again.
4. With “Do not ask again for this artifact” selected and confirmed, subsequent direct opens for the same viewer and artifact bypass the modal, including after reload. Other artifacts and viewers remain gated.
5. Toolbar, capability-banner, and device/location direct opens share the gate. Embedded iframe refreshes do not prompt or open extra tabs. External-link approvals keep their existing behavior.
6. Existing security/capability approvals and read-only recipient authorization are unchanged. Preference-storage failures do not break opening or silently disable the warning.
7. Add focused behavioral coverage for confirmation/cancellation, remembered and artifact/viewer-isolated choice, all direct-open entry points, and iframe reload behavior. Run appropriate existing suites and capture/update screenshots for the modal using available browser tools.
8. Close this ticket on the implementation branch before creating the PR; keep the main worktree clean on main and never push main.

