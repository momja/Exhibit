# Mobile text readability

Gallery cards show complete primary names on phones. Titles use `1rem`
(16px at the default preference), wrap even unbroken names, and grow their
metadata row. Desktop titles keep the existing 12.5px density and ellipsis;
the row uses a minimum height so enlarged text is not vertically clipped.

Creation descriptions, labels, gallery attribution/filter text, and shared
modal explanations use scoped relative sizing. Primary phone prose is 16px;
secondary prose is 14px. Form tokens retain separate source-code sizing and
fonts, and keep a 16px minimum on coarse pointers without disabling zoom.
The floating artifact header has no typography or geometry selectors changed.

Secondary, label, hint, placeholder, and error colors use shared tokens in both
themes. Placeholder opacity is explicit so browsers do not fade compliant text.
The browser regression checks 44 intended ink/surface pairs per theme, including
hover, ground, subtle, panel, selected, and feedback surfaces. Minimum measured
ratios are 4.513:1 light and 4.601:1 dark. Light muted text changes from 3.54:1
on white / 3.11:1 on ground to 5.83:1 / 5.11:1.

## Integration boundaries

The targets story owns 44px title and metadata hit areas. Preserve its
`min-height:44px` / `min-width:44px` on the title link when combining. This
story owns its `line-height:1.4`, `padding-block:6px`, font size and wrapping;
a 44px line height is not a reliable hit-size floor after typography overrides.
The metadata row's `min-height:26px` replaces the previous fixed height and
must remain free to grow around larger controls.

The creation story owns method-choice layout. Preserve or adapt the scoped
`.route b` / `.route-note` typography when combining its layout change. This
branch introduces its own `--text-primary` / `--text-secondary` tokens and
does not depend on any sibling branch.

The agent stylesheet also contains a focused composer text fix; it does not
change Chat/Preview segments. Sharing choice headings wrap within their cells.

After the main refresh, populated composer notices needed a shared height
limit. At 320×568, coarse touch and 200% root text, one saved `no-cors` warning,
one pending bearer connection request and three short input lines put Send's
bottom at 561.3px on main `9b8f08df`, 623.3px on readability `df667871`, and
682.3px on combined `92f3f095`. The first case fits the viewport; the latter
two clip. With two warnings and longer lines, main already clips too. These
comparisons used each pinned revision's actual templates and gallery assets,
with notices fetched from the saved artifact and connection-request partials.

Both htmx targets now sit in one scroll area using the previous notice height
cap. The composer can shrink its notice area around the enlarged input and
Send. Warning text and account actions remain reachable by scrolling; empty
notices reserve no space. The regression checks short and wrapped three-line
input, enabled and disabled agent layouts, native focus reaching both account
actions, control hit testing, retained warning text and empty notice spacing.
It is required by the browser CI execution gate.

[Before: readability `df667871`, enabled agent, 320×568, 200% text](screenshots/Exh-h9m4/composer-notices-before-320-200.png) ·
[After: same artifact, request, input and viewport](screenshots/Exh-h9m4/composer-notices-after-320-200.png)

## Verification

After the notice fix, `TestReadabilityBrowser`, `TestEnlargedFieldsBrowser`
and `TestEnlargedComposerNoticesBrowser` passed in 9.364s with no skips.

The final focused Chromium suite passed all eight selected tests (56.691s):

```sh
GOMAXPROCS=1 GOMEMLIMIT=384MiB go test -p 1 ./internal/api -count=1 -run '^(TestReadabilityBrowser|TestEnlargedFieldsBrowser|TestAppHeaderBrowser|TestGalleryFilterBrowser|TestGalleryActionMenuDuplicationBrowser|TestShareModalBrowser|TestAgentKeyModalBrowser|TestEditorLeaveModalBrowser)$'
```

`TestReadabilityBrowser` covers gallery/creation at 320, 375, 390, 430 and 1280px,
light/dark, at 100% and 200% root font size, with computed contrast, title/control
bounds, horizontal overflow and zoom checks. `TestEnlargedFieldsBrowser` covers
the real agent composer and sharing dialog at 200%, all four phone widths and
both themes. It checks line spacing and actual heading text-range bounds.

The six existing Playwright application journeys also passed (44.3s), using
isolated app/render ports 18484/18485 and scratch data; the temporary port
changes were restored. Initial `make build` passed, and the final gallery asset
regeneration and `go build -p 1` succeeded.

A separate local Playwright visual pass captured 64 layout states (gallery,
creation, Duplicate, Paste HTML and URL modes) with no horizontal page/dialog
overflow. Additional screenshots cover the composer and sharing fixes. Source
fields still compute to monospace at 32px on an emulated touch device at 200%.

The T3 collaborative browser captured the baseline; repeated transport failures
required switching to the repository's local Playwright workflow. Baseline
screenshots use a desktop pointer at phone width; final phone captures use a
coarse pointer, which also reveals the existing card action trigger.

[Gallery before](screenshots/Exh-h9m4/gallery-before-390.png) ·
[Gallery after](screenshots/Exh-h9m4/gallery-light-390-100.png) ·
[Dark gallery](screenshots/Exh-h9m4/gallery-dark-390-100.png) ·
[320px / 200%](screenshots/Exh-h9m4/gallery-light-320-200.png) ·
[Desktop density](screenshots/Exh-h9m4/gallery-light-1280-100.png)

[Creation before](screenshots/Exh-h9m4/new-before-390.png) ·
[Creation after](screenshots/Exh-h9m4/new-light-390-100.png) ·
[375px dark](screenshots/Exh-h9m4/new-dark-375-100.png) ·
[430px dark / 200%](screenshots/Exh-h9m4/new-dark-430-200.png) ·
[Enlarged composer](screenshots/Exh-h9m4/agent-dark-320-200.png)

Independent Standards review found no issues. Spec review found two P2 misses:
fixed composer line spacing and unbreakable sharing headings at enlarged text.
Both were fixed and independently verified at implementation commit
`f50e0f3077560c6bfe25b7792a1fd3883463e796`; no important findings remain.

These are Chromium viewport/coarse-pointer and root-font simulations, not
physical iPhone or VoiceOver testing. Full-suite runs interrupted by runtime
failures are not counted as passes; the parent runs aggregate checks after
combining stories. Safe-area rules and bar dimensions are unchanged here.
