---
id: av-p0a1
status: open
deps: []
links: [av-isb3, av-41se, av-hwx2]
created: 2026-07-18T15:15:41Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, edit]
---
# Edit page: manage network allowlist + capabilities (responsive)

Move all security *management* onto the existing Edit page so an artifact's allowlist, downloads, and clipboard are edited where its source is — and so the av-41se popover's "Manage" link has a destination. Because source can be large, the security section sits at the top in a collapsible panel. The Edit page today (edit.tmpl, web/gallery/edit.{css,js}, renderEditPage in gallery.go) already has title + CodeMirror + the save -> re-scan -> approve flow; this adds the security controls and a responsive layout.

## Mockups

Paper file 01KWQV7Y6G82HDZJ5CKPJTSJ7Y:
- Desktop: "Artifact Edit — B + dropdowns · CodeMirror".
- Mobile: "Artifact Edit — Mobile (collapsed)" and "Artifact Edit — Mobile (expanded)".
(The earlier bespoke-rail option "Artifact Edit — allowlist moved here (option)" is superseded by the collapsible layout — build from the B/CodeMirror + Mobile artboards.)

## Design

- Security at the top in a native <details>/<summary> panel; the summary shows the posture ("N origins · downloads: ask first · clipboard: ask first") and is collapsed by default so the editor stays reachable.
- Network allowlist: a plain list of approved origins, each with a Remove; plus an add-origin input. Origins the artifact references but has not approved (ingest-scan footprint minus NetworkAllowlist; scanner.Scan already runs on save) show as "referenced, not approved" rows with a one-click Allow. Nothing is auto-seeded into the allowlist from the scan. Rendered via html/template range (contextual auto-escaping).
- Downloads and Clipboard: native <select> bound to the EXISTING booleans — two options only: "Ask first" (flag=false) and "Always allow" (flag=true). No third "Block" state; no schema change. (downloads_approved / clipboard_approved are already PATCH-accepted in artifacts.go.)
- Source stays CodeMirror (the @codemirror/view island served at /assets/editor.js). Line-wrapping OFF so long / deeply-nested lines scroll horizontally; large files handled by CM virtualization.
- One Save -> PATCH /api/artifacts/:id carrying title, body, network_allowlist, downloads_approved, clipboard_approved (single write path). Editing the body re-runs the scan and re-gates the footprint via the existing showApproval flow.
- Mobile is CSS only — a breakpoint in web/gallery/edit.css, no separate template or JS branch. At narrow width: single column; the <details> body's two columns (allowlist | capabilities) stack; action buttons go full-width (Save primary, then Cancel/Delete). Native <select> yields the OS picker on mobile.

Files: edit.tmpl (security markup), web/gallery/edit.css (panel + @media), web/gallery/edit.js (fold in allowlist add/remove/allow from detail.js; wire selects), gallery.go renderEditPage view model (add Allowlist, the scan-diff, downloads_approved, clipboard_approved).

## Acceptance Criteria

- Edit page shows the collapsible security panel above the editor, collapsed by default with the posture summary line.
- Allowlist add/remove works; scan-flagged origins are approvable in one click; the allowlist is never auto-seeded from the scan.
- Downloads/Clipboard selects read and write the existing booleans (Ask first / Always allow), persisted via PATCH.
- A single Save persists title + body + allowlist + capabilities in one PATCH; changing the body re-runs the scan-approval gate.
- Long lines scroll horizontally in the editor; large files stay responsive.
- At <=640px the layout matches the Mobile mockups (single column, stacked security body, full-width actions) with no horizontal page scroll — verified on a narrow viewport.
- Origins are escaped by html/template.

No dependency — can start now (the badge and popover are independent of this page).
