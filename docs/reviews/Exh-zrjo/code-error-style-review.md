Pinned candidate: `6a18f7b09cf0711ad9d0112100bfb035f6c5a2eb`.

**Remaining actionable findings: 0.** The prior hygiene reuse P3 is resolved.

The unfiltered `907cb944...6a18f7b0` diff and the complete `internal web e2e cmd scripts Makefile go.mod go.sum` scope both contain exactly one change: `internal/api/templates/edit.tmpl:47` replaces `field-error` with `text-sm tag-error`. No additional application changes or exclusions were found.

The edit template loads `components.css` and `edit.css` at lines 13–14. Their existing `.text-sm` (`web/gallery/components.css:389`) and `.tag-error` (`web/gallery/edit.css:201`) rules supply the explanation's text size, error color, and top spacing. This matches the existing Tags errors in `internal/api/templates/partials.tmpl:163,173,183`, while eliminating the dependency on `.field-error`, defined only in the unloaded `web/gallery/new.css:113`. No CSS duplication or new abstraction is introduced.

The explanation remains template-owned, retains `role="alert"`, and stays outside the stable fieldset beside the declarative GET retry (`edit.tmpl:41–49`). This follows `docs/ui-components.md:3–5`: “Go owns markup and data.” Existing htmx, shared JavaScript, and component ownership are unchanged by this correction.

Read the original, follow-up, final, and hygiene briefs; all permitted original/prior reports and parent responses; and pertinent source/specification/standards references. Previously resolved behavior is supported by those reports rather than restarted speculative redesign.

Limits: read-only source inspection only. No builds, tests, servers, browser actions, source edits, commits, or subdelegation. Supplied validation is context; the reported full Go pass applies to `907cb944`, not an independently verified broad-suite pass on this style correction. Historical visual scores and actual BFCache behavior were not reassessed.
