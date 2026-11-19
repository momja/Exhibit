# Response to visual round 11

Sol scored all twelve categories 8. Opus confirmed the repaired selection colors,
but found that an opaque active-line fill covers the selection on that line.
It scored contrast and component consistency 7.5 in both themes. No joint pass
is counted; two fresh joint passing rounds are still required.

The symptom was reproduced on existing source by double-clicking a word. The
selection rectangle exists, but the active line paints above it. The new E2E
first failed on the old build with **identical selected and unselected paint**
(`rgb(240,243,248)` in light). This tests a missing selection indicator rather
than declaring a root color or a separately measured layer sufficient.

The active-line token now uses the same RGB palette at 25% opacity, allowing the
selection beneath it to remain visible. Both light and dark use this standard
layering treatment. Syntax colors, source contents, layout and saving behavior
are unchanged.

The new word-selection check double-clicks an actual comment word on the active
line, verifies the browser selection and active row, and composes the real
front-to-back background stack at that word. It requires a visible color change
and at least 4.5:1 text contrast. It covers Paste and an existing artifact editor
at 1280/390/320 through light → dark → light. The existing all-token and
placeholder checks now compose translucent fills, including the selected-active
overlap, instead of treating RGBA channels as an opaque background.

Capture automation adds six single-word selection states, for 80 states total.
The head line is visible in these images. The old build is saved separately in
`active-line-before/`; new candidate captures go in `visual-round-12/`.
The unchanged six-category rubric and independent light/dark scoring apply.
