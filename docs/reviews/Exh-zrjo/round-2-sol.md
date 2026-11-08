# Round 2 — independent Sol reassessment

Frozen build: `baf7a0c1a8a46c410d44fb767f276dae3e7ab68d` (HEAD verified). Read the [fixed brief](brief.md), both round-1 reports and [round-2 changes](round-2-changes.md). **Does not pass: workflow feedback remains below 8. No passing-round streak is established.**

## Evidence and limits

- **Observed live**, in my own T3 preview tab at 1280 and 320px: Library; blank Paste and URL validation; CodeMirror names/error association/focus; populated editor history; card menu and duplicate dialog (Escape cancellation and trigger-focus return); phone filter sheet; filter-miss recovery; viewer menu End/Escape behavior; phone viewer controls and hide/restore. No artifact, data, grant or permission mutations were submitted.
- **Observed saved images** in [round-2 screenshots](../../screenshots/Exh-zrjo/round-2/): desktop Library/Add/editor/viewer/filter miss; 390px dark Library/Settings/outgoing shares, light editor/incoming shares/search miss; 320px dark history/viewer and light/dark paste validation. These are authenticated app captures, unlike the original baseline attempt. The old reports' login-capture warnings remain valid historical records. The 36-capture manifest reports no errors; I did not independently recapture its full matrix.
- **Observed DOM enlargement:** at 320px with a 200% root font size, the editor heading grew to 42px, document width stayed 320px, and both populated history rows retained their 242px container width without internal overflow. Visible phone viewer buttons measured at least 44px in both dimensions; the restored-control tab measured 44×60px.
- **Source-reviewed:** ingest guards/locking/failure recovery, editor naming, relative typography, control/focus styles and viewer focus-exit handling. Reviewed the three UI-feedback E2E test implementations; their reported passing results and the wider validation log are parent-supplied evidence, not tests rerun by this reviewer. Creation/save success, network approval, refusal recovery, screen-reader speech, real touch hardware and a complete keyboard/contrast audit remain untested here. Provider execution/connectors remain outside scope.

## Scores

| Category | /10 | Rationale |
| --- | ---: | --- |
| Visual hierarchy | **8** | Visible Library context, readable card titles and prominent labelled desktop Add make the everyday path clear. Permission summaries are named and concise; clean Save is visually secondary. Desktop and phone captures retain useful spacing and readable surfaces. |
| Component consistency | **8** | Standard headers, button treatments, Phosphor icons, card surfaces and both palettes form a coherent system. The full-canvas floating viewer is a valid layout variant; its controls now share usable sizing and predictable Escape/focus return. |
| Navigation/discoverability | **8** | Card actions stay visible; Library return naming is consistent. Viewer identity is visible, desktop Edit is direct and phone Edit is available in More. Phone filters expose the fixture's collection/tags; filter recovery returned to the populated Library. Duplicate is reachable and cancels safely. |
| Workflow feedback | **7** | Field errors now identify/focus the relevant input, result counts have live semantics, and empty states offer recovery. Pending locking/retry recovery is supported by source and supplied regression evidence. However, validation simultaneously announces false progress: see blocker below. |
| Accessibility | **8** | Actual contenteditables have distinct HTML source, Artifact source and Gallery widget source names. Paste errors associate through `aria-describedby` and `aria-invalid`; keyboard menu/dialog checks passed. Shared focus treatment, stronger control boundaries and responsive text enlargement address the earlier substantial access barriers. This is a focused review, not a conformance certification. |
| Responsive behavior | **8** | Representative 390/320 light/dark captures fit coherently. Live 320px Library, filter sheet, duplicate and viewer remain usable; history reflows even at enlarged text. Viewer targets and restore affordance meet the checked 44px phone size. Artifact-authored canvas styling is excluded. |

## Blocking finding

**P2 — `/new`: invalid submissions leave a persistent false upload status.**

**Observed:** choose Paste with empty source, then Add to library. Focus correctly moves to the named source textbox and `#body-error` says “Paste an artifact first.”, but `#status[role=status]` simultaneously says **“Uploading…”**. Switching to URL clears feedback, but submitting its empty field again leaves “Enter a URL first.” alongside “Uploading…”. The same contradiction is visible in both `add-error-320-*.png` captures. Controls remain editable; this is not an actual pending request.

**Source cause:** [new.js](../../../web/gallery/new.js#L157) writes progress before validation; the returns at lines 163, 166 and 172 happen before the request and pending lock. The polite live region can therefore announce activity that never starts. This defeats the required clear distinction between validation failure and in-flight work, despite the improved field-level treatment.

**Concrete fix:** validate/build the payload first, then set progress immediately before locking and sending the request. Leave progress empty on validation failure (or use one accurate error summary). Extend the existing validation regression to assert an empty/non-progress `#status`, no API request and enabled retry controls for blank Paste, blank URL and malformed URL. Retain the pending/refusal checks.

## Remaining refinements and next round

Phone editor Save remains a named icon-only control; a short visible “Save” label would improve scanning. Add's header and larger “Three ways in.” heading still repeat context, but are understandable. Neither is a substantial blocker in this build.

Fix and independently recheck the false-progress state on the next frozen build. A future all-8 round can establish only the first passing round; completion still requires both reviewers to pass all six categories twice consecutively and the required regression checks to finish successfully.
