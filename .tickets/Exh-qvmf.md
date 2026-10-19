---
id: Exh-qvmf
status: closed
deps: []
links: []
created: 2026-10-19T19:41:26Z
type: feature
priority: 2
assignee: Max Omdal
---
# Artifact favicon with logo E and tag dots

Show the E extracted from the Exhibit logo with up to three vertically stacked tag-color dots on sandbox viewer pages and direct render documents. With no tags, show only the E.


## Notes

**2026-10-19T19:45:57Z**

Implemented shared SVG artifact favicon using the logo E and up to three normalized tag colors. Viewer pages and direct/version/share renders use it; render-time tokenization replaces stored favicon links while preserving source bytes and scripts. Built assets, visually checked 16px/32px light/dark variants, and validated focused tests and go build ./.... Full suite validation pending.

**2026-10-19T19:51:52Z**

Refined the dots to overlap in a centered vertical stack, per user clarification. Rechecked actual generated SVGs at 16px and 32px on light/dark backgrounds. Updated geometry tests, artifact/render favicon tests, API favicon tests, and go build ./... pass.

**2026-10-19T19:58:59Z**

Final verification: make assets, go build ./..., full internal/render suite, updated artifacticon/render favicon tests, and focused API favicon tests passed. The first complete API run passed (194s). Repository-wide go test ./... passed every other package but hit an intermittent API failure during concurrent server test activity; a captured full API retry then passed with zero failed tests. Corrected overlapping dot stack is committed in b75ee8c.
