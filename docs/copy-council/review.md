# Copy council review

Work: [Exh-iust](../../.tickets/Exh-iust.md). Full [page-by-page change inventory](changes.md).

## Acceptance rule

Independent Codex Sol (`gpt-6.1-sol`) and Claude Opus (`claude-opus-5-5`) reviewers rate necessity, clarity, and hierarchy from 0–10. Both reviewers must give every category at least 8 in two consecutive rounds. Reviewers were asked to preserve security and irreversible-action meaning and to report concrete objections rather than raise scores to meet the target.

Scope: Library/search/empty states; creation; viewer/editor/agent; sharing, duplication, downloads and permissions; Settings, connected agents and connections; sign-in, OAuth and error/render fallback pages. User artifact content, API-only errors, operator diagnostics, developer docs and agent system prompts are excluded.

## Scores and evidence

| Round | UI revision | Reviewer | Necessity | Clarity | Hierarchy | Outcome |
| --- | --- | --- | ---: | ---: | ---: | --- |
| 1 | `a6682df4` | Sol | 6 | 6 | 6 | Revise |
| 1 | `a6682df4` | Opus | 5 | 5 | 6 | Revise |
| 2 | `d7ac2662` | Sol | 8 | 7 | 8 | Revise |
| 2 | `d7ac2662` | Opus | 7 | 7 | 8 | Revise |
| 3 | `5ba4f080` | Sol | 8 | 8 | 8 | Sol qualifies |
| 3 / 3b retry | `5ba4f080` | Opus | — | — | — | Provider usage limit; no review |
| 4 | `0a2cb08e` | Sol | 8 | 8 | 8 | Second consecutive Sol pass |

**Joint council acceptance is pending, not passed.** Opus's last actual scores remain 7/7/8 from round 2. Its round-3 attempts returned no scores. The raw provider notice states: “You've hit your 5-hour limit · resets 7:50pm (UTC)” on 2026-11-04. T3 marked both attempts failed. No alternative-model rating is substituted for the requested Opus review.

Reports: [Sol R1](round-1-sol.md), [Opus R1](round-1-opus.md), [Sol R2](round-2-sol.md), [Opus R2](round-2-opus.md), [Sol R3](round-3-sol.md), [Sol R4](round-4-sol.md).

Baseline review was source-based. Subsequent reviews inspected source diffs and rendered desktop/390px phone screenshots in [screenshots](../screenshots/Exh-iust/). Reviewers did not claim live browser inspection. Parent browser checks use the actual isolated app with scratch sample data.

## How findings were resolved

- Removed introductory and repeated copy, shortened panel explanations, standardized Library / artifact / Saved data / Permissions / Card widget names, and made source/description/widget instructions persistent.
- Fixed inaccurate refetch undo wording, current-data erasure/history scope, permanent Connections clearing, future external-link permissions, sharing writer/reader audiences, and token library scope.
- Added an explicit Give access submit action, grouped imports/exports with their explanations, put sharing disclosures at the shared dialog level, and retained account limits under a concise disclosure.
- Replaced unavailable recovery actions with truthful connected-preview and Connections empty-state guidance. SSO failures now render a plain explanation with Return to sign in, retaining technical diagnostics in logs and the original refusal behavior.
- Sol R3's localized widget-help/import-failure/“personal grant” findings were resolved before R4. R4 reported no remaining copy blockers.
- A final disabled-widget-generation tooltip was aligned with the reviewed unavailable-agent wording while updating existing test expectations; this tooltip-only cleanup follows the reviewed UI revision.

## Delegated task records

All IDs have prefix:
`node:delegated-task:command%3Amcp%3A91d7c77a-a4ba-4ee7-9e6c-24215dcc31ad%3Adelegate-task%3A`

Suffixes: `exh-iust-copy-sol-r1`, `exh-iust-copy-opus-r1`, `exh-iust-copy-sol-r2`, `exh-iust-copy-opus-r2`, `exh-iust-copy-sol-r3`, `exh-iust-copy-opus-r3` (usage limit), `exh-iust-copy-opus-r3b` (usage limit), `exh-iust-copy-sol-r4`.

## Outstanding work

After Opus access resets, review the final source and saved screenshots with Opus, resolve any remaining objections, and continue both reviewers until **two consecutive joint passing rounds** exist. Do not treat the two Sol passes as joint acceptance. Keep the ticket in progress until this condition is met.

## Validation

- `make build` completed with all embedded frontend assets rebuilt.
- Actual browser checks: Give access added a scratch recipient and refreshed the server-rendered list without restarting the artifact; no horizontal overflow at 390px. Creation also checked at 320px and 1280px, and Settings, the library, and the not-found page at 390px; all had no document overflow.
- Desktop/phone screenshots cover creation, sharing, Permissions, data transfers, connected-agent token scope, Connections, agent preview, 404, and the final widget instructions.
- Existing assertions were updated to the new labels and permission explanations. Added regression coverage for SSO failure recovery without a raw diagnostic and for protected-preview guidance that does not prescribe a disabled new-tab remedy.
- Short-test coverage completed across all packages. Long combined invocations ended with SIGTERM (exit 143), so validation was completed in bounded package/test batches rather than reported as a successful uninterrupted `go test -short ./...` run.
- All 864 top-level API tests were assigned to four exhaustive, disjoint `go test -short -count=1 -p 1 ./internal/api -run ...` batches of 216. Batches 1, 2 and 4 passed. Batch 3 reported only an outdated assertion expecting the removed “pi binary” tooltip; that assertion was corrected and `TestPlatformModeEnablesWidgetGeneration` then passed. The remaining batch-3 tests, including `TestGalleryPageScriptSuite` (the gallery Node regression suite), completed without failures.
- All 245 top-level store tests passed in two exhaustive batches (123 and 122 tests). The other package checks passed; `internal/tile`, after the interrupted combined run, was checked separately and passed. Normal `-short` skips still apply to optional browser/integration tests.
- `git diff --check` passed. Main remains clean; all development is on the feature worktree. The ticket remains in progress solely because the joint Opus/Sol scoring condition is not complete.
