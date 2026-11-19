# Planned response to focused baseline32 (not implemented)

Keep the candidate frozen until BOTH independent baseline reviews are terminal.
Opus32 reports7.5–9.5, strongest blockers in clarity/feedback. Sol32 is still
active at last actual task_status; its current peer scores remain withheld.

Priorities for the source change, to reconcile with Sol's independent findings:

1. Place attachment/removal errors beside their controls, preserving their text
through the htmx replacement. Do not keep an enabled Add action with no available
tag. Explain that adding/removing tags saves immediately.
2. Explicit library-wide scope in Edit tag, named Save tag, near-field Name/hex
validation with aria-invalid/descriptions/announcement/focus and dirty/valid Save.
Maintain cancellation, pending locks, working-copy recovery and focus restoration.
3. Keep color selection distinct from keyboard focus. Give presets human-readable
names, keyboard-friendly single-selection semantics and a visible Custom state.
Use enough room for an intact color dot and the inset/concentric check at every
width; do not shrink the dot/check to evade the geometry defect. A consistent
4-column44px palette is a candidate, to verify visually against actual shapes.
4. Phone tag rows should share a label/action grid, with compact pills and fixed
right-aligned44px controls. Strengthen name emphasis, quiet utility icons and
outline very dark dots with the existing token. Preserve desktop density.
5. Replace the native Delete confirm with a clearly named in-sheet confirmation
step, keeping artifacts and scope explicit. Use a footer whose DOM and displayed
order match across sizes; preserve shared tokens and native modal lifecycle.

Prefer one shared color-field partial and the existing Stimulus color-picker for
local working copies; tags.js owns API calls/events only, htmx owns server markup.
No user-authored artifact changes. Parent persistence/error/pending/retry checks
must use disposable fixtures, not the current Run Log tags. Update/strengthen
existing behavior tests and add actual browser regressions only where meaningful.
All old asserts must be inspected and preserved in intent, never dropped to pass.

Before evidence already saved: tags-ten-before/8actualT3captures, currentaa7,
actuallight/dark1280/390. After source changes, show matching real before/after
through html_preview then html_render before next council. No mock image substitute.
No passing-suite rerun on unchanged source; new workflow changes will need build,
relevant Node/native/E2E checks, focused current captures, then one final Go suite.
