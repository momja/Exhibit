# Round 22 response to both independent round 21 reviews

Frozen application `17baf9306f455dbca4882a468e8ba2afe088436f`, served assets
`f1b6e9fd582c`. Both R21 reviews were terminal before this source was changed.
Sol light/dark[8.5,8.5,9,9,8.5,9]; Opus light/dark[9,9,9,9,9,8.5]. Joint fail;
current target9 joint streak0. These repairs still need fresh independent scores.

## Share starts at its beginning

The People input no longer has autofocus. Native dialog focus now lands on the
existing first header control, Close, so opening does not silently scroll to a
text field or request typing. On a successful new opening, the visible scroll
body is explicitly reset to0; existing working-copy/pending/htmx behavior stays
in the existing shared modal and share module. A repeated show on an already
open/pending dialog does not reset an active interaction. Focus still returns
to Share on dismissal, and a deliberate tap/click reaches the People field.

Parent reproduced old320×640 actual light/dark scrollTop351/client477/height1124,
active share-add-input. Current first opens compute scrollTop0 and active header
Close in both. The regression additionally scrolls, closes and reopens the
sheet to catch retained scroll. First old-binary run failed at351 as expected.
An intermediate attempt reset scroll while hidden; the reopen check found633,
so reset moved to successful opening. No assertion or timeout was weakened.

The meaningful E2E now passes32 first/reopen states across320×640,390×640,
640×360 and1280×800, both actual schemes and independent fine/coarse pointer
contexts. It confirms header focus, top notice, deliberate input focus and
return focus. Touch is Chromium emulation; physical software-keyboard behavior
remains inferred rather than certified. No OS-keyboard claim is made.

## Comfortable editor explanations

Stored state/assets introductory, deletion, empty/loading and export/import
paragraphs use the existing form-help token:13/19.5 desktop,14/21 phone. The
shared details-panel supplies12px paragraph separation, including nested
server-rendered asset rows. Versions now uses the same component rule rather
than a private copy. The app-owned Gallery widget explanation uses it too;
user-defined keys and source/widget designs are untouched. Text/actions remain
intact. Parent measured all5 Stored assets paragraphs14/21 with12px gaps at320
in both schemes; previously12/normal and0px gaps.

Small R21 residues addressed: Embed tile helper now uses form-help; desktop
Create and add tag is44px like Add; the desktop card posture trigger has a real
minimum32×32 target without negative margin, beside the32px menu. Phone44px
minimums remain. Repeated public-link guidance inside the expanded switch
explanation, native dark radio styling and the consistent fact-eyebrow versus
field-caption distinction remain minor judgment calls, not automatic exemptions.

## Verification and evidence

Build passes;468 Node tests pass16.435s on final JS. All17 E2E pass1.1m,
including real source paint/chrome, actual Share scrolling/clipping and the new
opening/reopen journey. Final focused Tag/Share/Filters/navigation/phone-target
Chromium passes15.307s. The original Go browser focus expectation now points to
Share's header Close rather than People, preserving the actual native focus
trap, cancellation/reconnection and return-focus checks. Full current Go short
and220-state authenticated capture refresh follow on this frozen candidate.
R21 fullGo had confirmed exit0 in456.95s/applicationUnchanged=true on74544552;
that prior result is retained as historical, not substituted for current checks.

Actual320×640 before/after in both schemes shown via html_preview then
html_render, saved nine-opening-comparison.html. Sources in nine-r22-before/
and nine-r22-details/. Capture workflow now explicitly adds expanded state,
assets/widget app help and short/landscape Share views with opening assertions;
previously overlooked surfaces must not silently disappear from coverage.

No persisted sample mutations. One Chromium engine/emulated viewports, not
physical phones, alternate engines or hosted identity. Tests do not determine
visual ratings; every material app-owned surface remains independently scoreable.
