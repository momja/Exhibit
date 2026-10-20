---
id: Exh-zlsl
status: closed
deps: [Exh-poaf]
links: []
created: 2026-10-20T06:24:35Z
type: feature
priority: 1
assignee: Max Omdal
parent: Exh-xvtt
---
# Artifact download choice modal

Offer Everything (.zip) and Artifact HTML only (.html) from every existing artifact download action. Use native dialog with Stimulus and shared template/styles, native modules and embedded Phosphor. Cover cancellation, keyboard/focus, retries, desktop/mobile and update screenshots.


## Notes

**2026-10-20T06:37:57Z**

Implemented the shared native download dialog and Stimulus controller for gallery menus and artifact viewer controls. ZIP is recommended/default; HTML remains explicit. Uses api-module apiFetch and downloadBlob, server-rendered choices, pending guards, error/retry, focus restoration and disconnect cleanup. Added real Chromium coverage for htmx swaps/appends and mobile, template/read-only checks, and four app screenshots. Shared bundle schema and docs/artifact-bundles.md are unchanged. Focused UI tests and a real signed-in HTML download passed; full repository tests are running.

**2026-10-20T06:43:13Z**

Final validation: make assets passed; go test ./internal/api -run Test\(ArtifactDownload.*\|GalleryActionMenuDuplicationBrowser\|ActionMenuBrowser\|DetailPageMenuFollowsReadOnly\)$ -count=1 passed with real Chromium (17.399s). A signed-in real-app HTML download completed and restored focus to pip-more. Four desktop/mobile gallery/detail screenshots are committed. go test ./... was interrupted by the turn cancellation before completion, so no full-suite pass is claimed. ZIP response bytes are fixture-backed in the UI browser test until the parallel exporter is integrated; no export API or shared contract changes were made.
