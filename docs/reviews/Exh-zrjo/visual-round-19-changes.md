# Round19 response: real modal scrolling

Application `a65d7c6bb4cb1277c5c44c51de4db6293ada2b0c`, assets `b1ef638450cc`.
Both R18 executions are terminal. Sol independently found the same Share
containment failure as the parent: the fieldset body reports overflow but
refuses to scroll. It rated light/dark typography9, composition7, harmony9,
contrast8, component consistency7, responsive polish7. Opus produced no scores
because its five-hour quota resets19:50UTC Nov4. Neither is a joint9 pass.

## Change

Duplicate, Edit tag and Share now use a regular `.modal-body` block as the
constrained flex scroll container. Their semantic `.modal-fields` fieldsets
live inside it. Share's stable `#share-body` fieldset and htmx swap target,
native disabled/pending behavior, focus restoration and all feature actions
remain. The block establishes the positioning/paint context; positioned
radios and preset badges stay with their cards within the scrolling region.
The fixed header/footer stay separate. No further type, palette or copy change.

Parent actual390px evidence in both real schemes: scrollTop0 before versus389
after the same downward wheel gesture. Both complete saved-data choices,
public-link rule and switch disclosure are reachable, while Done remains at
y784–828 and body bottom763. The detached radio no longer paints in the footer.
Actual before/after was checked with html_preview and shown with html_render;
saved in `nine-scroll-comparison.html` and `nine-scroll-before/after` captures.

## Regression evidence

The new E2E first fails the actual old binary: body scrollTop remains0 and
offscreen paint is not owned by the footer. It now passes on this application
at320/390, light/dark: actual scroll movement, footer hit-testing over a clipped
radio, stable footer, and access to the final expanded disclosure paragraph.
An initial test selector also matched Embed tile; it was narrowed to the saved-
data disclosure's exact existing ID. This was a test ambiguity, not a UI fix.

The two prior source-matrix timeouts passed on unchanged234da736 when rerun
after concurrent heavy work ended (2 tests,17.7s). No timeout was enlarged and
no assertion removed. The full Go run failed four stale markup expectations
after the intentional R18 grammar change; expected new classes/heading/targets
are updated, and the header icon count is scoped to the header so new dialog
Close buttons do not affect it. All four focused checks pass1.307s. Full
candidate revalidation is tracked in `nine-validation.md`.

The capture workflow now verifies a real wheel scroll and saves Share's lower
settings at1280/768/390/320 in both actual schemes, rather than photographing
only the newly fixed footer's top view. No persisted fixture changes.

## Retained choices and open coverage

Everything accepted/observed in R18 remains scoreable, including fixed240px
rail, quiet source echoes/selection hierarchy, whole-unit2+1 Find options at320,
necessary four-line Share warning, arbitrary color/long-data limits and sampled
native completion coverage. See all earlier reports and R18 response.
Repairing this defect is not an automatic9; fresh independent scores remain
required. No qualifying joint9 streak exists. Opus needs a fresh post-reset
execution on the same application if Sol's review and current source qualify.
