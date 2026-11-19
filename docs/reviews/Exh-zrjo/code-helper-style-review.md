Pinned candidate: `8945098046dbff6c31e8e5c4309fb5e5bd5c94d0`.

**Remaining actionable findings: 1.**

- **P3 — Helper composition overrides the Tags error color.** `internal/api/templates/edit.tmpl:47` now uses `form-help tag-error`. The shared helper rule at `web/gallery/components.css:455` uses `:is(.modal-check small,.form-help,...)`, which takes specificity `(0,1,1)` from `.modal-check small`. Its muted color therefore beats `.tag-error` `(0,1,0)` at `web/gallery/edit.css:201`, even though the edit sheet loads later. The explanation loses the error color supplied by the prior `text-sm tag-error` version. Make the existing error-color override specific enough for this helper composition, retaining the shared helper typography and paragraph spacing. This is a concrete component-composition regression, not a new abstraction or visual rerating.

The unfiltered `6a18f7b0..89450980` diff changes exactly `text-sm` to `form-help` on that paragraph. The unfiltered and complete application-scope `907cb944..89450980` diffs each contain only the same paragraph's original `field-error` replacement. No other application change was found.

The page loads both relevant stylesheets (`edit.tmpl:13–14`). Shared `components.css:251,455` supplies the 12px bottom gap and helper typography; `tokens.css:169` makes that typography 14px on phones. The stable fieldset, alert role, template-owned explanation, and declarative GET retry remain unchanged (`edit.tmpl:41–49`).

Limits: source inspection only; no builds, tests, servers, browser actions, source edits, commits, or subdelegation. Full-Go context applies to `907cb944`; no broad-suite pass on this candidate is claimed.
