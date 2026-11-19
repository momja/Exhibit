# Before-fix observations

The initial reproduction used the previously built application from `f58de10b`
(current evidence-only HEAD was `26dae10d`) in the repository's isolated E2E
server. Exec session 52628 actually reported:

- Tag mutation: while the fragment GET was held, `#tag-add-name` was enabled
  instead of disabled.
- Pending import: Replace all changed `<h1>Keep my working copy</h1>` to
  `<h1>Changed my working copy</h1>` despite `contenteditable="false"`.

Those are observations from the recorded tool result; their original ephemeral
trace directory was overwritten by later E2E runs. They are not fabricated
standalone exit receipts. The third test in that initial invocation failed
because the test tried reading a response body after navigation; that harness
failure is not counted as a confirmed application defect.

The attempted real BFCache check did not obtain a cache restoration. Chromium's
`notRestoredReasons` reported `masked`; its diagnostic is retained separately.
The explicit persisted-event test then failed on the old build, waiting for a
reload that never happened. The corrected candidate passes that event test and
ordinary Back navigation, while preserving unfinished network approval. Actual
BFCache restoration remains unverified.

The first cleanup candidate `2e2fc9bc` introduced two regressions that the fresh
subagent reviews caught. New browser checks actually failed on that candidate
for native color synchronization, expired-session recovery and 404 rendering;
full raw log and exit1 receipt are saved in
`code-validation-review-regressions-before.txt` and its status JSON. The
corrected candidate `8a4c0fe1` passes these checks.

The first broad Node invocation omitted `--experimental-vm-modules`, so its
module-test harness failed. The corrected command passes 479 tests. Broad Go and
E2E runs on the superseded cleanup were deliberately terminated after review
found regressions; their negative/143 exits are retained and are not passes.
