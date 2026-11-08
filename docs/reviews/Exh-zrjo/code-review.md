# Code simplicity review

Reviewed the cumulative application diff atop PR273 (`ab0135c9`) through
`26dae10d`, then repaired and reviewed the code-quality follow-up through
`742301c5`. Three fresh Codex subagents independently reviewed standards,
behavior/spec, and component ownership. Original and subsequent findings are
preserved, including regressions found during cleanup.

Tag mutations keep a native fieldset locked through the server-rendered htmx
refresh. A template-owned Retry issues only the partial GET; htmx retains its
normal session-recovery and error-fragment behavior. CodeMirror owns its
read-only state through a narrow handle. The picker owns color presentation;
menu owners share keyboard movement; palette names/colors have one definition;
modal helper typography comes from shared components.

The final follow-up composes existing helper typography/spacing with Tags error
color. Actual dark-mode inspection confirmed 14px text, 12px before the focused
Retry control, locked stale fields and successful GET-only recovery. No
historical visual rating is reassigned to this code-quality candidate.

## Standards

Remaining actionable findings: **0**. Severity: **none**.

Reviewed pinned candidate `907cb944a9a296cd0aae3dd59fb739fa1f22c365`, the actual two-file `8a4c0fe1...907cb944` delta, the complete cleanup `26dae10d...907cb944`, relevant originating cumulative paths, all original/prior reports and responses, and the supplied standards/specifications. The previous zero-finding reports apply to their earlier pin; this conclusion includes the follow-up.

- `internal/api/templates/edit.tmpl:48` now composes the existing `.btn` and `.btn-sec` rules (`web/gallery/components.css:14–18`), removing the undefined modifier without adding page-specific styling or behavior. The retry remains template-owned and outside the locked fieldset (`edit.tmpl:41–50`). This follows `docs/ui-components.md:3–5`: “Go owns markup and data.”
- `internal/api/gallery_test.go:111–113` adds the native-fieldset assertion and corrects the trigger expectation to include the declarative GET retry. The existing GET and tag-control assertions remain. The test now matches the template's stable wrapper and both triggers; no production interaction changes here.
- The surrounding ownership remains sound: `web/gallery/tags.js:44–99` locks through refresh, resolves replacement controls, allows normal session/error-fragment handling, and exposes only GET recovery. This follows `docs/technical_stack.md:459–476`: “Resolve elements on use,” with normal htmx error responses. The dialog stays outside the swapped body.
- Earlier cleanup findings remain source-resolved: the picker owns synchronization/validation presentation; CodeMirror owns read-only facets through its narrow handle; completed-import recovery preserves unfinished approval; shared menu movement, palette entries, and modal typography avoid the earlier duplication. These satisfy the documented boundaries (`docs/ui-components.md:253–262,564–571,813–819`).

No further concrete simplicity smell was confirmed. Smell assessments are heuristic judgments, subordinate to repository rules; tooling-covered style was excluded.

Limits: read-only source inspection only. No builds, tests, servers, browser actions, source edits, subdelegation, or current hygiene-phase peer reports. Supplied validation is context, not independent verification. Actual BFCache restoration, live focus/render behavior, and historical visual scores were not reassessed.

Original findings and responses: [initial](code-initial-standards.md), [cleanup](code-cleanup-standards.md), [earlier verification](code-final-standards.md).

## Spec

Remaining spec findings: **0** — missing/partial requirements: 0; unrequested behavior: 0; incorrect implementation: 0.

Reviewed fixed candidate `907cb944a9a296cd0aae3dd59fb739fa1f22c365`, the actual `8a4c0fe1...907cb944` delta, complete cleanup changes and relevant originating cumulative source, against the original briefs, commit history, all original/prior reports and parent responses, ticket acceptance/history, focused Tags brief, product requirements and pertinent repository standards. Historical visual ratings were not reassessed.

The later validation correction is appropriate: `internal/api/gallery_test.go:112–113` adds the native-fieldset assertion and updates the exact trigger expectation to include the existing declarative GET retry. Existing assertions remain. `internal/api/templates/edit.tmpl:48` now composes the established `.btn`/`.btn-sec` styles (`web/gallery/components.css:17`); the change adds no interaction path or mutation retry.

The relevant cleanup remains source-resolved:

- The stable fieldset stays locked through mutation and refresh (`edit.tmpl:41`; `web/gallery/tags.js:44–81`). Refresh tags requests the same partial GET; transport failure keeps stale controls disabled, and 5xx first swaps server content before offering recovery.
- No response cancellation suppresses `HX-Refresh`; the 404 fragment has no controls and guarded synchronization clears local feedback safely (`tags.js:75–99,144–147`). This preserves the stack requirement that an expired session reloads into login and “htmx 4 swaps error responses” (`docs/technical_stack.md:469–476`).
- Native color input copies its originating value before the controller's common validation/synchronization path (`web/gallery/color-picker-controller.js:15–27`), satisfying the documented “updates both inputs” contract (`docs/ui-components.md:259`).
- CodeMirror owns both read-only state and editability behind the narrow handle (`web/editor/editor.js:139–181`); located page consumers use it. Persisted-page recovery is gated on completed ingestion, preserving unfinished network approval (`web/gallery/new.js:331–354`).

Limits: read-only source and regression-test-source inspection only. No builds, tests, servers, browser actions, source edits or current-phase peer reports. Supplied validation was context, not independent runtime verification. Actual BFCache restoration and live focus/native-dialog/error outcomes were not exercised.

Original findings and responses: [initial](code-initial-spec.md), [cleanup](code-cleanup-spec.md), [earlier verification](code-final-spec.md).

## Component ownership and reuse

The functional ownership findings were resolved. The last reuse pass then found
an unloaded error class; replacing it exposed a helper-color specificity issue.
Both were fixed and verified against the actual cascade and rendered view.

Pinned candidate: `742301c5a8270cfeb242db300d009f0829ee9ad0`.

**Remaining actionable findings: 0.**

The helper-composition color regression is source-resolved. `web/gallery/edit.css:201` retains `.tag-error` and adds `.form-help.tag-error` to the same existing declarations. The composed selector has specificity `(0,2,0)`, beating the shared helper rule's `(0,1,1)` (`web/gallery/components.css:455`). The refresh explanation therefore receives the existing danger token without duplicating helper typography.

Shared helper sizing and line height remain owned by `components.css:455` and `tokens.css:139,169` (13px desktop, 14px phone, line height 1.5). The shared paragraph rule still supplies its 12px bottom gap (`components.css:251`); the error rule supplies its existing 8px top margin. This follows the documented `p.form-help` composition (`docs/ui-components.md:225–229`).

The plain `.tag-error` selector and declarations are unchanged, preserving the three existing `text-sm tag-error` feedback elements (`internal/api/templates/partials.tmpl:163,173,183`). The added intersection currently matches only the refresh explanation (`internal/api/templates/edit.tmpl:47`); no unrelated component or selector dependency was introduced.

The unfiltered `89450980..742301c5` diff changes only that CSS selector list. The complete unfiltered `907cb944..742301c5` diff contains exactly the explanation's class replacement and this selector adjustment. Alert semantics, fieldset, declarative GET retry, JavaScript, functional Go code, and tests are unchanged.

Limits: read-only source inspection only. No builds, tests, servers, browser actions, commits, or subdelegation. Parent validation is context; no broad-suite pass on this style candidate or new visual rating is claimed.

Preserved reports: [initial](code-initial-reuse.md), [cleanup](code-cleanup-reuse.md),
[functional verification](code-final-reuse.md), [error-class finding](code-hygiene-reuse.md),
[intermediate correction](code-error-style-review.md), [cascade finding](code-helper-style-review.md).

## Validation and limits

[Validation details](code-validation.md) distinguish exact checked revisions:
full Go on 907; full 29 browser and479 Node on 8a; final template/native and four
Tag browser checks on 742. Only error-feedback styling changed after the full
Go pass. [Runtime identity](code-runtime-identity.json) retains all later diffs,
current complete scope fingerprint, and matching preview executables.

[Before-fix observations](code-before-observations.md) retain failures and harness
limits. [Healthy-view captures](../../screenshots/Exh-zrjo/code-simplification/manifest.json)
are pinned to 8a; [error-feedback comparison](../../screenshots/Exh-zrjo/code-simplification/refresh-comparison.json)
compares 907 and 742 with a simulated offline GET. Source reviewers did not run
checks themselves. Actual BFCache restoration was not obtained; ordinary Back
and the explicit persisted-page event are covered. The feature stays stacked
on PR273; the main worktree was kept clean/on main.

Original findings: Standards 4 (worst P3); Spec 1 (worst P2); reuse 2 (worst P2).
Final unresolved findings: Standards 0 (none); Spec 0 (none); reuse 0 (none).
