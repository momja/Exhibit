# Response to round 2

The fixed scope and rubric are unchanged. Both reviewers scored 8/8/8/**7**/8/8 and identified the same remaining blocker: invalid Paste/URL submits falsely announced “Uploading…”. No passing streak has been established.

## Fix

`ingest()` now builds and validates the complete payload before setting progress and taking the pending lock. Blank Paste, blank URL and malformed/non-HTTP URL leave the live progress status empty; the single field-associated error remains. Agent validation follows the same rule. Valid submits still announce upload/snapshot work immediately before the network request.

The E2E validation test now checks all four invalid states, an empty progress region, zero artifact POSTs and enabled retry/method controls. The pending test still gates the server response, submits repeatedly, confirms only one request, and verifies editable working-copy recovery after refusal. All other app behavior and design are unchanged from round 2.

## Evidence

- Current screenshots: `docs/screenshots/Exh-zrjo/round-3/` (same 36-state authenticated capture matrix).
- Compare `round-2/add-error-320-light.png` with `round-3/add-error-320-light.png`: the false progress panel is gone; only the actual inline error remains.
- Round 2's broad real-Chromium matrix passed (81.1s), all 459 Node tests passed, and 9 E2E tests passed. The strengthened E2E suite is rerun for this candidate.
- A full Go short run compiled an earlier revision of one CSS-source assertion and failed only that obsolete assertion; its corrected form passed the focused browser/contract run. The full suite is rerunning on this final candidate; see `validation.md` for the completed result.

Please independently reassess every category and the formerly blocked validation state. A target score is not a reason to inflate ratings. If both reviewers now give all categories >=8, this establishes only the first passing round; a fresh confirmation round is still required.
