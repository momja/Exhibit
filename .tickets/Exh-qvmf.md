---
id: Exh-qvmf
<<<<<<< HEAD
status: closed
=======
status: in_progress
>>>>>>> 500dc0a (Start Exh-qvmf artifact tag favicon)
deps: []
links: []
created: 2026-10-19T19:41:26Z
type: feature
priority: 2
assignee: Max Omdal
---
<<<<<<< HEAD
# Artifact favicon with balanced Mondrian tag colors

Show a Mondrian mosaic inspired by the Exhibit logo on sandbox viewer pages and direct render documents. Use up to three tag colors in gallery order in similarly sized panels, with a smaller white panel and offset black dividers. Untagged artifacts use the Exhibit brand palette; unused tag panels stay white.


## Notes

**2026-10-19T19:45:57Z**

Implemented shared SVG artifact favicon using the logo E and up to three normalized tag colors. Viewer pages and direct/version/share renders use it; render-time tokenization replaces stored favicon links while preserving source bytes and scripts. Built assets, visually checked 16px/32px light/dark variants, and validated focused tests and go build ./.... Full suite validation pending.

**2026-10-19T19:51:52Z**

Refined the dots to overlap in a centered vertical stack, per user clarification. Rechecked actual generated SVGs at 16px and 32px on light/dark backgrounds. Updated geometry tests, artifact/render favicon tests, API favicon tests, and go build ./... pass.

**2026-10-19T19:58:59Z**

Final verification: make assets, go build ./..., full internal/render suite, updated artifacticon/render favicon tests, and focused API favicon tests passed. The first complete API run passed (194s). Repository-wide go test ./... passed every other package but hit an intermittent API failure during concurrent server test activity; a captured full API retry then passed with zero failed tests. Corrected overlapping dot stack is committed in b75ee8c.

**2026-10-19T21:53:20Z**

Replaced the E-and-dots favicon with the user-approved Mondrian design. Balanced tag panels now occupy 49, 48, and 42 square units (about 35%, 35%, 30% of colored area). Fixed grid and tag order retain visual memory. Untagged artifacts use the brand palette; unused panels stay white. Saved the generated SVG preview screenshot. All artifacticon tests, focused render/API favicon tests, go build ./..., and git diff --check passed.
=======
# Artifact favicon with logo E and tag dots

Show the E extracted from the Exhibit logo with up to three vertically stacked tag-color dots on sandbox viewer pages and direct render documents. With no tags, show only the E.

>>>>>>> 500dc0a (Start Exh-qvmf artifact tag favicon)
