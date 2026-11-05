# Round18 candidate: shared visual grammar

Application `234da736cec712dc1d1f50e4f8ae0871e4ed4bff`, assets `8309e86eb320`,
based on origin/main `44def51a`. Both round17 reviews finished before edits.
This is a candidate response to their target9 critique, not a passing claim.

## Response to both round17 reports

- **Forms and type:** sentence-case 600-weight field captions, explicit helper
  size/line-height, upright placeholders,18px native-semantic checkboxes and
  consistent44px standard controls. Compact desktop row actions are36px.
  Settings fact keys remain metadata; admin and Storage values share weight.
  Page titles share26px/650 type and24px desktop/28px phone top spacing.
  Add has one visible page heading and a Library return, matching Settings.
- **Dialogs:** Duplicate, Edit tag and Share share a fixed header/footer and
  scrolling body. Existing native modal/focus/pending behavior stays intact.
  Desktop primary actions trail; phone<=400px actions stack full width in
  primary/secondary/destructive order. Edit tag Delete has a quiet outline.
  Full-width buttons center labels, including Filter Done and editor Return.
- **Share:** one visible saved-data/account warning, short section helpers and
  stronger section heads. The distinct rules about public links, personal data,
  shared data and connections remain explicit. Replace/Revoke are44px; grant
  rows no longer reserve an unused15px scrollbar gutter. Done stays available
  while the body scrolls. Radio choices use visible native choice indicators
  on phones. No sharing defaults, grants or saved-data behavior changed.
- **History and source chrome:** desktop Versions actions use an intrinsic
  track and stay side by side; phone grouping remains. Current has an accent
  badge and the explanatory copy uses the helper rhythm. Find uses a flexible
  field plus fixed Close, whole option units wrap, both native Close controls
  share the same border/surface, completion rows center their content and
  tooltips keep an8px viewport inset. Real selection/active/echo paint and
  syntax palettes are preserved. Parent caught a remaining320px Previous
  word split after the first pass; navigation now allocates40% to Previous
  and30% each to Next/All, wrapping whole controls under enlarged text.
- **Library and color:** title first-line, tag dots, posture and kebab share
  optical alignment; incoming owner attribution follows the same card inset.
  Search is a visible desktop rail destination with an active marker. Phone
  header gaps are consistent. The miss helper is shorter/balanced and Clear
  uses a44px phone target. The light selected Add method has a persistent tint
  and inset ring. Pure-gray secondary neutrals move into the existing slate
  family; the blue brand and coherent dark palette remain.
- **Tag colors:** preset suggestions use a restrained palette; existing user
  colors/default normalization remain unchanged. Selected presets and custom
  colors are visible. A shared Stimulus color-picker owns only local working
  state; tag API saving and htmx rendering remain feature/server-owned.
  The asset build explicitly includes the controller and self-hosted info icon.

## Deliberate retained choices and limits

The240px desktop rail remains: both reviewers found readable two-column cards
at768px and Sol called further rail work optional. Source secondary hints stay
quiet to preserve the verified selection hierarchy and text contrast. No new
brand, decorative content or product workflow was added. Arbitrary user tool
bodies and credential-dependent execution remain outside this visual scope.
None of these decisions exempts a material weakness from a fresh score.

## Verification and evidence

- Asset builds and editor lint pass. Final incremental editor/gallery/icon
  build produced the served asset8309e86eb320; no concurrent asset rebuild
  during Go compilation.
-468 gallery Node tests pass. An initial invocation lacked the documented
  vm-modules flag; a later failure exposed a missing fake-DOM controller target
  and Event global in the fixture. The fixture and custom refresh event are
  corrected; the actual shipped controller is also browser-tested.
- Real Chromium dialog/header/space/Search/tag/icon group passed54.621s
  before the final isolated Find layout correction. Shared modal reference
  assertions now compare the matching shell and body/helper roles. New Tag
  coverage exercises preset/custom sync, isolation from underlying creation
  fields and reconnection after an actual htmx swap.
-14 E2E passed1.8m before the isolated Find correction. The final candidate
  run is in progress, with a strengthened rendered single-line navigation
  assertion at1280/390/320 in both editors and schemes. Cancellation coverage
  includes the new Close and its pending-save lock.
- Full final Go short suite and refreshed authenticated capture matrix are
  running; consult `nine-validation.md` for actual completion, not this note.
- Actual320px Tag before/after in both real schemes was shown through a checked
  T3 HTML comparison. Saved in `nine-first-pass-comparison.html` with source
  captures in `nine-before-details/` and `nine-first-pass-details/`.
  Broader final captures: `docs/screenshots/Exh-zrjo/nine-round-18/`.

Parent rendered checks: Tag320 light/dark selected Health and no horizontal
overflow; Share320 light fixed Done; Find320 dark field194px/Close44px/gap8,
whole option units and240px retained source; desktop Versions light/dark View
and Restore on one row (36px controls,60px old-version row versus96px before).
The final Previous word-split correction is verified by the strengthened E2E
and fresh captures. These checks are evidence, not council scores.
