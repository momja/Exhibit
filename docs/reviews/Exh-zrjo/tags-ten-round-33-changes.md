# Focused tag UI response after baseline32

Both independent baseline reviews completed before source changes. R32 Sol scores
8.5–10, Opus7.5–9.5; no focused passing round. The new UI must earn its ratings.

- Shared color controls now use a4×2 palette of56px cells with complete22px
  samples and the existing16px check/inset3/outer8/inner4 geometry. The first44px
  revision still collided at320; the actual regression caught it, and56px passes.
  Never shrink away the sample or restore a badge that touches the outline.
- Named radios, one tab stop, arrows/Home/End and synchronized native/hex inputs;
  a selected-color caption says Gray/Green/Custom without impersonating focus.
  Invalid hex is described/announced beside its field and blocks Save tag.
- Compact13px pills with dark-dot separation and quieter32px desktop/44px phone
  utilities. A single form-width list gives every row aligned actions.
- No enabled, impossible Add when every tag is attached. Attach/remove errors
  now sit next to their controls, with retained text restored after htmx swaps.
- Tags explains immediate add/remove persistence; Edit tag explains library-wide
  scope. Name errors are next to Name, announced, marked invalid and focused.
  Save tag is disabled while clean/invalid and says Saving… while locked.
- A separate body Delete entry opens a same-sheet Keep/Delete confirmation with
  artifact-retention copy. Cancel/Save and Keep/Delete footers follow actual DOM
  order at all widths. Returning to a fresh confirmation clears old server errors.
- Existing native modal focus/dismissal/pending locks, library-wide APIs and
  server partial/htmx ownership remain. No user-authored artifact changes.

Meaningful proof: new geometry assertion FAILS old1280 UI; an intermediate44px
revision FAILS320. Both receipts remain saved. Actual disposable-tag E2E verifies
rename/recolor on two artifacts, add/remove, pending single-write locks, client
and server validation, save/delete failure and retry, clean state and focus;
no persistent fixture is altered. Current focused3E2E passed34.02s before the final
copy/confirmation-reset refinement. Current full suite will provide final evidence.
Native Tag modal actually ran both1280/390 subtests and passed; no skip inferred.

The current candidate and assets will be supplied with the next round. No test
result or prescribed layout is automatic10. Inspect real renders independently;
mandatory16-view Tags/sheet matrix is narrow and does not reopen broad pages.
