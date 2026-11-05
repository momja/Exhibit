# Copy council review

Work: [Exh-iust](../../.tickets/Exh-iust.md). Full [page-by-page change inventory](changes.md).

## Acceptance rule

Independent Codex Sol (`gpt-6.1-sol`) and Codex Astra (`gpt-6-astra`) reviewers rate necessity, clarity, and hierarchy from 0–10. The user explicitly substituted Astra for unavailable Claude Opus on 2026-11-05; earlier Opus reviews remain historical evidence. Both reviewers must give every category at least 8 in two consecutive rounds. Reviewers were asked to preserve security and irreversible-action meaning and to report concrete objections rather than raise scores to meet the target.

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
| 6 | `b2f8def2` | Sol | 8 | 8 | 8 | First joint Sol/Astra pass |
| 6 | `b2f8def2` | Astra | 8 | 8 | 8 | First joint Sol/Astra pass |
| 7 | `80c48ae6` | Sol | 8 | 8 | 8 | Second joint Sol/Astra pass |
| 7 | `80c48ae6` | Astra | 8 | 8 | 8 | Second joint Sol/Astra pass |
| 8 | `66e95a57` | Sol | 8 | 8 | 8 | Final verification; third joint pass |
| 8 | `66e95a57` | Astra | 8 | 8 | 8 | Final verification; third joint pass |

**Accepted: three consecutive joint Sol/Astra passes, including final UI revision `66e95a57`.** Both reviewers independently scored necessity, clarity, and hierarchy 8/10 in rounds 6, 7, and 8. Neither found remaining acceptance blockers or concrete regressions in the final verification. Earlier Sol-only passes do not count as joint acceptance. Opus's last actual scores were 7/7/8 from round 2; later Opus attempts returned no scores due a provider quota on 2026-11-04. On 2026-11-05 the user authorized replacing Opus with Codex Astra.

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

## Completion

All requested copy changes, location inventory, rendered evidence, relevant checks, and independent council verification are complete. The ticket is closed on the feature branch. The final [interactive summary](summary.html) contains the complete inventory grouped by page; [changes.md](changes.md) retains source links for every group. Main remains clean and on `main`; changes have not been merged into it.

## Validation

- `make build` completed with all embedded frontend assets rebuilt.
- Actual browser checks: Give access added a scratch recipient and refreshed the server-rendered list without restarting the artifact; no horizontal overflow at 390px. Creation also checked at 320px and 1280px, and Settings, the library, and the not-found page at 390px; all had no document overflow.
- Desktop/phone screenshots cover creation, sharing, Permissions, data transfers, connected-agent token scope, Connections, agent preview, 404, and the final widget instructions.
- Existing assertions were updated to the new labels and permission explanations. Added regression coverage for SSO failure recovery without a raw diagnostic and for protected-preview guidance that does not prescribe a disabled new-tab remedy.
- Short-test coverage completed across all packages. Long combined invocations ended with SIGTERM (exit 143), so validation was completed in bounded package/test batches rather than reported as a successful uninterrupted `go test -short ./...` run.
- All 864 top-level API tests were assigned to four exhaustive, disjoint `go test -short -count=1 -p 1 ./internal/api -run ...` batches of 216. Batches 1, 2 and 4 passed. Batch 3 reported only an outdated assertion expecting the removed “pi binary” tooltip; that assertion was corrected and `TestPlatformModeEnablesWidgetGeneration` then passed. The remaining batch-3 tests, including `TestGalleryPageScriptSuite` (the gallery Node regression suite), completed without failures.
- All 245 top-level store tests passed in two exhaustive batches (123 and 122 tests). The other package checks passed; `internal/tile`, after the interrupted combined run, was checked separately and passed. Normal `-short` skips still apply to optional browser/integration tests.
- `git diff --check` passed. Main remains clean; all development and ticket closure are on the feature worktree.

## Resumed council — 2026-11-05

The user requested Codex Astra instead of Opus. Joint round 6 reviews the unchanged final interface at `b2f8def2` with independent Codex Sol and Codex Astra reviewers. Both tasks are read-only and inspect the prior findings, responses, final source, and saved desktop/phone evidence. Acceptance still requires two consecutive joint rounds with every score at least 8.

Round-6 task IDs:
- Sol: `node:delegated-task:command%3Amcp%3Af06d7919-58d9-4e1e-a1fd-c28129115a57%3Adelegate-task%3Aexh-iust-copy-sol-r6`
- Astra: `node:delegated-task:command%3Amcp%3Af06d7919-58d9-4e1e-a1fd-c28129115a57%3Adelegate-task%3Aexh-iust-copy-astra-r6`


### Round-6 outcome and corrections

Both reviewers scored necessity, clarity, and hierarchy 8/10. This is the first qualifying joint Sol/Astra round. Reports: [Sol R6](round-6-sol.md), [Astra R6](round-6-astra.md).

Before confirmation, addressed all findings: artifact-specific Connections only suggests available steps; recipient redirect warnings defer to the owner; post-import review guidance is direction-neutral; preparation noise is hidden in recorded as well as live conversations; connection actions/notices and restoration feedback use consistent nouns/verbs; removed the final decorative 404 caption. Added coverage of unavailable/available connection setup and recipient redirect explanations. The final text inventory includes these changes.

Round-7 preparation validation: `make build` and focused Go checks for connection availability, connection actions, transcript projection, not-found rendering, and `TestGalleryPageScriptSuite` passed. The last includes gallery Node regressions, including the two new recipient-redirect cases. Actual-app screenshots now cover artifact-specific Connections (desktop/phone), 404 without decorative captions, and a recipient redirect warning. The redirect screenshot uses a controlled fixture message from the displayed frame's identity to exercise the real notice handler; it does not claim an actual external redirect. Recipient `READ_ONLY` was true and the notice no longer directs the recipient to Edit → Permissions. No phone document overflow was detected.

Round-7 task IDs:
- Sol: `node:delegated-task:command%3Amcp%3Af06d7919-58d9-4e1e-a1fd-c28129115a57%3Adelegate-task%3Aexh-iust-copy-sol-r7`
- Astra: `node:delegated-task:command%3Amcp%3Af06d7919-58d9-4e1e-a1fd-c28129115a57%3Adelegate-task%3Aexh-iust-copy-astra-r7`

These independent confirmation tasks review UI revision `80c48ae6`. No scores are assumed before their actual results return.

### Round-7 outcome and final corrections

Both reviewers again scored necessity, clarity, and hierarchy 8/10, establishing the second consecutive joint passing round. Reports: [Sol R7](round-7-sol.md), [Astra R7](round-7-astra.md).

Both noted a competing **Open in new tab** action on redirected-request warnings; it is now hidden for that failure because a new tab cannot bypass network permissions. The action remains visible for sandbox limitations it can resolve. Sol also found an editor status incorrectly pointing below its network review region; it now says **“Saved. Review network access before leaving.”** Both were localized nonblockers at the qualifying scores; a final round will verify this cleanup against the complete interface.

Final-cleanup validation: `make build` and `go test -short ./internal/api -run '^TestGalleryPageScriptSuite$'` passed. Extended existing network regressions to cover owner/recipient redirects hiding the ineffective action and ordinary preview limitations retaining it. Actual-app checks caught and corrected a shared CSS display rule overriding the button's hidden state; the final phone and desktop checks confirm computed `display: none`, zero button width, and no horizontal overflow. Screenshots: [phone](../screenshots/Exh-iust/round-8-recipient-redirect-phone.png), [desktop](../screenshots/Exh-iust/round-8-recipient-redirect-desktop.png). These use the same controlled fixture message described above, not a real external redirect.

### Round-8 final verification

Both independent reviewers verified the two final corrections at `66e95a57` and again scored every category 8/10. Reports: [Sol R8](round-8-sol.md), [Astra R8](round-8-astra.md). This is the third consecutive joint passing round; no further copy change or council round is required. Remaining explanation density was noted as optional refinement, not a concrete regression or acceptance blocker.

Round-8 task IDs:

- Sol: `node:delegated-task:command%3Amcp%3Af06d7919-58d9-4e1e-a1fd-c28129115a57%3Adelegate-task%3Aexh-iust-copy-sol-r8`
- Astra: `node:delegated-task:command%3Amcp%3Af06d7919-58d9-4e1e-a1fd-c28129115a57%3Adelegate-task%3Aexh-iust-copy-astra-r8`
