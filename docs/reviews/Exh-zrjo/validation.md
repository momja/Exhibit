# Validation log

## Baseline

- `make build`: passed.
- `go test -short ./...`: passed (API package 462s).
- Real Chromium: AppHeader, MobileTargets, GalleryFilter, GalleryEmptySpaces: passed.

## Improved build

- Asset build: JS syntax checked; CodeMirror, Phosphor, htmx and Stimulus rebuilt.
- Gallery Node suite: 459 passed, none skipped (`--experimental-vm-modules`).
- `npm test --prefix e2e`: **9 passed** (6 existing duplicate/state tests and 3 new UI feedback regressions).
- Focused real-Chromium + contract run (creation chooser/method switching, gallery space navigation, mobile targets, card/header assertions): **passed**, 36.6s.
- `node e2e/capture-ui.mjs docs/screenshots/Exh-zrjo/round-2`: **36 authenticated states captured**, no document overflow or browser errors; see manifest.
- Broader real Chromium matrix: **passed**, 81.1s. Covers all standard/auth/floating headers, gallery navigation/filters/empty spaces, 320–430px targets, creation chooser/method drafts, editor navigation, readability, enlarged fields, and modal interface.
- Strengthened round-3 E2E suite: **9 passed**, 18.9s. Invalid agent/paste/empty-URL/malformed-URL submissions leave progress empty; zero artifact POSTs; retry remains enabled.
- Round-3 capture: **36 states**, no document overflow or console errors.
- `go test -short ./...` on the final candidate: **passed**, exit 0 (API package 418.4s; store 188.9s). All Go packages passed.

## Final evidence

- [Full Go output](validation-go.txt)
- [End-to-end browser output](validation-e2e.txt)
- [Broad Chromium output](validation-chromium.txt)
- [Round-3 capture manifest](../../screenshots/Exh-zrjo/round-3/manifest.json)

The first broad runs hit a disk-space limit and one overlapped asset rebuild.
Those runs are not counted as passes. A later full run used an obsolete source
assertion compiled before its correction; that failure was corrected and the
final full run above passed. The application candidate remained unchanged at `006b14ef` throughout
rounds 3 and 4. Later changes are review reports, documentation, screenshots
and ticket closure only.
