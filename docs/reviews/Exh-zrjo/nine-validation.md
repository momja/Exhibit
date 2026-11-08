# Target9 final validation

Runtime application `d30dbbe92f9f5535a33f59be25f7c9f9c4be4511`, served assets
`e29fcce0e122`, base origin/main44def51a. BOTH25 and BOTH26 reviewed this unchanged
interface. The only later application-path diff is validation-only commita787bd5e
in **internal/api/gallery_test.go**. Exact path scope is verified in the final Go
receipt; no production source/asset/API/behavior change.

| Check | Final actual result | Evidence |
| --- | --- | --- |
| Gallery assets / Go binary | Pass before frozen UI reviews | /tmp/exhibit-zrjo-r25-build.log |
| Gallery Node / editor lint |468 pass16.435s / lint pass, carried on unchanged productionJS/editor code | nine-validation-node.txt and prior lint receipt |
| Full browser journeys |18 pass, exit0/78.53s, applicationUnchanged=true | nine-r25-e2e-status.json; nine-validation-e2e.txt |
| Native origin drafts/navigation/permission saves/Tag/Share/phone targets | Exit0/24.32s wrapper; package21.790s, applicationUnchanged=true | nine-r25-focused-status.json; nine-validation-focused.txt |
| Affected markup expectations after test-only fix | Both pass0.028s | nine-validation-permissions-expectations.txt |
| Standard full Go short | Exit0/423.55s, API410.584s/store182.020s | nine-r25-go-confirm-status.json; nine-validation-go-status.json/txt |
| Authenticated captures |244 verified, exit0/312.98s, applicationUnchanged=true | nine-r25-capture-status.json; ../../screenshots/Exh-zrjo/nine-round-25/manifest.json |
| Council ratings |48 source-table values9 in two consecutive joint rounds25/26 | nine-final-rating-verification.json; original reports |

No current check remains pending. No passing suite/capture was repeated merely
for a docs/evidence/test-expectation-only change. No asset rebuild during Go.

## What the current browser regression establishes

The existing editor-tools journey compares rendered permissions headings/captions/
button paint with matching shared tag/field roles in actual light/dark at all four
widths. The old binary fails12/700/normal/muted versus14/600/21px/group ink.
Current checks preserve Tag chip32/44 targets, aligned property-row heights,
boolean44 target, phone primary width, transfer grouping, native Edit/Cancel/focus,
Move/Cancel and no persisted state change. Local Add/Remove origin drafts trigger
real htmx swaps: the empty helper disappears and returns; saved allowlist stays
empty. The populated phone address line is above its action and fits its row.
No assertion or timeout was weakened. Fixtures are disposable and cleaned.

Capture verification checks244 manifest entries and PNG files, errors[], every
versioned assete29fcce0e122, and actual media.dark agreement with requested scheme.
The capture workflow asserts document horizontal containment before every shot.
It adds8 explicit permission views to the prior236-state capture matrix. Actual
before/after permission views at1280/320 bothschemes are in nine-r25-before/details;
html_preview had no console errors and html_render displayed the comparison.

## Failed/incomplete attempts retained and resolved

- Initial full18 and focused-Go unified wrappers terminated143. The E2E log had
  seven successful cases; focusedGo had a25.058s success line but no reliable
  child exit receipt. Neither was counted as the final validation pass.
- The first detached E2E attempt exited1/1.32s before tests because the interrupted
  runner left its scratch server on18080/18081. Verified node server.mjs101855 and
  child101866 with DATA_DIR /tmp/exhibit-e2e-REPiXl; SIGTERM to that wrapper cleaned
  only its scratch instance. Preview18280/18281 remained untouched. Fresh isolated
  run passed without reuseExistingServer. Receipts/logs: nine-r25-initial-validation.json
  and nine-r25-e2e-port-blocked-status.json/txt.
- Original fullGo exited1/451.04s with TWO stale gallery_test.go markup expectations:
  Blocked still required old text-sm/muted h3 classes; External links omitted the
  shared field-label class. Failed output/receipt remain in nine-validation-go-r25-failed
  files. AFTER both25 terminal, a787bd5e updates the explicit h3/caption expectations
  and comments. Labels, IDs, bootstrap/grant assertions and behavior remain checked.
  Focused0.028s then full standard423.55s confirmation both pass.

The current Go receipt records applicationUnchanged=true,
validationOnlyChanges=[internal/api/gallery_test.go], validationChangeScopeExpected=true,
and validation-only commita787bd5e. Runtime proof excludes ONLY that exact test
file; all other internal/web/e2e runtime differences must be empty. Current
canonical nine-validation-go files hold the final result. Prior R24 canonical
files are archived as nine-validation-go-r24.txt and nine-validation-go-r24-status.json.
R23/R24 and earlier source-specific passes remain historical, not substitutes.
Earlier capture wrapper143/late-child/confirmed retry and stale Erase expectation
records are preserved in their original files.

## Limits and procedural record

One Chromium engine with emulated viewports and actual schemes, not physical
software-keyboard/phone, alternate-engine, hosted-identity or exhaustive
accessibility certification. Populated asset/referenced/blocked origin rows,
arbitrary long/custom data and uncommon source variants remain sampling limits.
Each reviewer records exact live versus unseen states. User-authored tool/widget
bodies and credentialed execution are excluded. Tests did not determine scores.

Sol26's accidental historical aggregate exposure is retained in its report and
nine-round-26-scope-audit.json. Public timing and tracked commit prove no current
same-round peer result existed in the compared material; previous-round25 scores
were already provided in the brief. No private deliberation was copied into the
audit. All temporary quota-resume schedules were permanently deleted; final list[].
