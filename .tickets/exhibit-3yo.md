---
id: exhibit-3yo
status: closed
deps: []
links: []
created: 2026-07-02T06:50:31Z
type: epic
priority: 2
---
# [epic] Artifact body editing on /artifacts/<id>/edit

Umbrella for the artifact body-edit experience on /artifacts/<id>/edit. Children share the same edit surface, form/API write path, and re-scan flow, so they should be built against a common structure rather than duplicated. Scope: (1) CodeMirror 6 source editor for the body field; (2) re-scanning the edited body and surfacing newly-detected network origins for approval.



## Notes

**2026-10-19T19:50:26Z**

Audited on 2026-10-19 against origin/main 550499c. Both scoped features are shipped:

1. [exhibit-ay7](exhibit-ay7.md): CodeMirror 6 source editor, implemented in 5c45fbb and merged as a1f4254. web/editor/editor.js mounts the HTML editor and synchronizes its contents to the body field consumed by the existing edit/PATCH path. The bundled editor is served as an embedded asset.
2. [exhibit-2hl](exhibit-2hl.md): body-edit network re-scan and explicit origin approval, already implemented under [Exh-4021](Exh-4021.md) (cef31e9 / merge 13fa43d). Closed the stale child after verifying the API and edit-page approval flow.

Validation: CodeMirror bundle and gallery asset builds succeeded; targeted API/scanner/store Go tests passed (167 cases including subtests), all 13 edit-page JavaScript tests passed, and an additional approval-flow smoke check passed. Checks ran in a temporary export of the audited revision, leaving application code untouched.

No feature work remains in this epic; closing as completed.
