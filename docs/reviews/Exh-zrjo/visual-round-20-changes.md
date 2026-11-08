# Round20 response: shared history columns

Application `8504bc516e4be2ed7fd8369b7f3823632cc6272f`, assets `33556024040a`.
Both round19 phases are settled: Sol completed; Opus was never launched before
the known quota reset and will not review the superseded application.

Sol19 independently gave typography/harmony/contrast/component consistency/
responsive polish9 in BOTH schemes, composition8.5. Share scrolling is genuinely
fixed, including lower choices and expanded disclosure, all four widths/both
schemes. The remaining material issue was desktop Versions: each row sized its
action cell independently, so Current versus View/Restore displaced identical
timestamps by108.453125px.

The list now provides shared intrinsic grid tracks; desktop rows use subgrid.
The date and trailing-action cells share the widest required track across rows.
No fixed176px action width returns, and View/Restore stay on one line. Existing
phone grouping/reflow stays independent; the previous per-row layout is the
progressive fallback for engines without subgrid. No other app UI changed.

The existing history/enlarged-phone E2E now measures date/action x positions
across three actual versions, at1280/768 in light/dark, while retaining all its
320px enlarged-text and viewer-control checks. It first failed the actual old
binary with108.453125px date drift, then passed current(3.0s). No test timeout or
assertion was relaxed. Parent live1280 bothschemes confirms both dates at
x715.90625, action cells x859.6484375 and older View/Restore on one row.

Actual before/after details from1280px screenshots were checked/shown through
T3 HTML and saved in `nine-history-comparison.html`; full original screenshots
remain. No synthetic app render or data mutation was used for that comparison.

Current full E2E/Go/capture outcomes belong in `nine-validation.md`. The prior
current Go attempt ended with signal143 before writing a result record; no
assertion failure or reason was supplied, and it is not a pass. The final
standard Go short check will use a detached runner with durable status.

All previous history remains required context. In particular: generous240px rail,
quiet source hints/selection hierarchy, whole-unit320 Find options, necessary
Share warning and sampled native completion/detail coverage remain scoreable.
R19 Sol freshly verified the8px completion inset and centered rows. The timestamp
fix does not automatically earn9; fresh independent scores and two actual joint
passing rounds are still required. Target9 qualifying joint streak is0.
