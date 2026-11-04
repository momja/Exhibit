---
id: Exh-iust
status: in_progress
deps: []
links: []
created: 2026-11-04T15:35:00Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, copy, ux]
---
# Refine interface copy with Codex Sol and Claude Opus council

Audit Exhibit interface text for necessity, clarity, and hierarchy. Use independent Codex Sol and Claude Opus reviewers each round; revise copy and placement until all three categories score at least 8/10 from both reviewers in two consecutive rounds. Record every text change and its location.

## Acceptance Criteria

All categories score at least 8/10 from both reviewers in two consecutive rounds; relevant checks pass; rendered desktop and phone evidence and a complete change summary are saved; main remains clean.


## Notes

**2026-11-04T16:18:19Z**

Implemented copy and placement changes in feature/Exh-iust/interface-copy, with complete inventory at docs/copy-council/changes.md and source/screenshot review evidence at docs/copy-council/review.md. Sol scores reached 8/8/8 in rounds 3 and 4. Opus last scored 7/7/8 in round 2; round 3 and retry failed at its five-hour usage limit (resets 2026-11-04 19:50 UTC). Joint two-consecutive-round acceptance remains outstanding, so this ticket stays in progress. Final validation is being completed in bounded API batches after the long test process ended with SIGTERM.

**2026-11-04T16:28:47Z**

Validation completed: asset/server build and diff check passed; all API test cases covered by four exhaustive short-test batches, with the only stale-tooltip assertion corrected and its targeted rerun passing; gallery Node suite passed; all store tests covered by two passing batches; other package checks passed. Live Give access submission preserved the running artifact; responsive checks and final screenshots saved. Long combined test invocations ended with SIGTERM, so results are recorded as bounded batches rather than a successful uninterrupted full invocation. Implementation and inventory are ready; only joint council acceptance remains blocked by Opus usage quota.
