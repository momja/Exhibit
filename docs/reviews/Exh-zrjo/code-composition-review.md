Pinned candidate: `742301c5a8270cfeb242db300d009f0829ee9ad0`.

**Remaining actionable findings: 0.**

The helper-composition color regression is source-resolved. `web/gallery/edit.css:201` retains `.tag-error` and adds `.form-help.tag-error` to the same existing declarations. The composed selector has specificity `(0,2,0)`, beating the shared helper rule's `(0,1,1)` (`web/gallery/components.css:455`). The refresh explanation therefore receives the existing danger token without duplicating helper typography.

Shared helper sizing and line height remain owned by `components.css:455` and `tokens.css:139,169` (13px desktop, 14px phone, line height 1.5). The shared paragraph rule still supplies its 12px bottom gap (`components.css:251`); the error rule supplies its existing 8px top margin. This follows the documented `p.form-help` composition (`docs/ui-components.md:225–229`).

The plain `.tag-error` selector and declarations are unchanged, preserving the three existing `text-sm tag-error` feedback elements (`internal/api/templates/partials.tmpl:163,173,183`). The added intersection currently matches only the refresh explanation (`internal/api/templates/edit.tmpl:47`); no unrelated component or selector dependency was introduced.

The unfiltered `89450980..742301c5` diff changes only that CSS selector list. The complete unfiltered `907cb944..742301c5` diff contains exactly the explanation's class replacement and this selector adjustment. Alert semantics, fieldset, declarative GET retry, JavaScript, functional Go code, and tests are unchanged.

Limits: read-only source inspection only. No builds, tests, servers, browser actions, commits, or subdelegation. Parent validation is context; no broad-suite pass on this style candidate or new visual rating is claimed.
