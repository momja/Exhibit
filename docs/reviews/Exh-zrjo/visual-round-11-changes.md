# Response to visual round 10

Sol scored light/dark contrast 7.5 and the other ten scores 8. Opus hit its
provider usage limit and produced no scores. No joint pass is counted.

The complete light syntax palette is restored, but the focused selection layer
still used CodeMirror's stock fills. The app's selector was less specific than
the library's focused-selection hierarchy. `Prec.highest` could not overcome that
specificity difference. Real selected text therefore rendered against `#d7d4f0`
in light and `#3e4451` in dark; Sol measured small syntax tokens below 4.5:1.

The theme selector now matches the actual focused scroller/selection-layer
hierarchy so the app fills win. Parent live checks on existing source confirmed
`rgb(219,231,251)` in light and `rgb(39,54,74)` in dark. Syntax styles, palette,
layout, sharing and application behavior are otherwise unchanged.

The old E2E created a probe from the intended root token, which could not detect
the cascade failure. That assertion was replaced with **actual focused source
selection** using Control/Meta+A, then the computed selection-layer background.
The strengthened check first reproduced the failure on the old candidate. It
now covers both Paste and an existing artifact editor at 1280px, 390px and 320px,
through light → dark → light. Each sampled HTML/CSS/JavaScript token must retain
at least 4.5:1 against the field, active line and rendered selection backgrounds,
and the unsaved draft must survive. The test does not accept a color declaration
as evidence of the final painted background.

The full E2E suite passed all 12 tests (23.5s). The capture workflow now adds six
actual source-selection states, for 74 captures; it clears the temporary Paste
draft after each capture. Current images go in `visual-round-11/`, preserving
the previous evidence. The same rubric and threshold still apply. Two fresh
joint passing rounds are required, with Opus resuming after its 09:50 UTC reset.
