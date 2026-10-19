# Agent readiness stack review

Review and merge from bottom to top. Each PR targets the previous branch so its Files changed view isolates one concern. No branch has been merged to main.

| Order | PR | Change | Review focus |
| --- | --- | --- | --- |
| 1 | [#169](https://github.com/momja/Exhibit/pull/169) | Pi 1.3.0 | Runtime engine, RPC, usage, guardrail, old session compatibility |
| 2 | [#170](https://github.com/momja/Exhibit/pull/170) | Source reuse prompts | Read on missing/stale/uncertain context; successful own saves need no reread |
| 3 | [#171](https://github.com/momja/Exhibit/pull/171) | Source/widget OCC | Independent immutable revision tokens, atomic checks, no-op and quota races |
| 4 | [#172](https://github.com/momja/Exhibit/pull/172) | Cached source and targeted edits | Local match resolution, exact patch spans, no hidden reads or rebases |
| 5 | [#174](https://github.com/momja/Exhibit/pull/174) | Eager session/source preparation | Typing debounce, lifecycle races, structured roles, persistence and freshness |

## End-to-end contract

Three non-whitespace characters sustained for 300 ms start one scoped session and an authorized source read. Draft text stays in the browser, and no provider request runs before Send. Send captures the accepted request and claims the pending or ready session. Unclaimed sessions have a 90-second server deadline as well as browser cleanup. Abandoned History requests rely on that server lease, since another tab can claim even a newly created sidecar before its response arrives. Rollback has a bounded wait and reports uncertainty without automatically retrying a mutation.

The first accepted provider request includes an assistant `get_artifact` call with a matching real tool result. Raw source, title, state, widgets and selection remain data in tool-result roles. Pi custom data entries persist prepared reads; the context hook reconstructs structured pairs for resume and compaction. Per-session server credentials remain restricted to the authenticated owner and one artifact.

Only source actually displayed to the model seeds the edit cache. Successful writes advance its body and revision. Targeted edits resolve fuzzy matches locally and send exact changed spans with an expected revision. Source and widget writes use independent tokens; conflict invalidates the relevant cache and never silently rebases. Unchanged turns reuse retained source and known successful writes without another full-source projection.

## Review and validation

Every PR has passed a separate read-only review-agent review with no remaining findings. Pi, prompt, and patch reviews passed their first round; OCC and eager preparation were reviewed again after fixes.

| PR | Reviewed implementation | Result |
| --- | --- | --- |
| #169 | `6d893ee` (same patch as reviewed `888aba0`) | No findings |
| #170 | `38f2011` (same patch as reviewed `b371766`) | No findings |
| #171 | `ac5debf` (same patches as reviewed `c558b60`) | No findings after snapshot/no-op, quota ordering, and failed-write retry fixes |
| #172 | `d04bbe6` (same patch as reviewed `26f791c`) | No findings; targeted extension checks passed after replay |
| #174 | `4aadf77` | No findings after startup/rollback deadlines, cache/compaction, shared-session cleanup, and composer integration fixes; 127 focused tests passed in the final individual review |

The stack was rebased onto main at `beefacf`. Range-diff verified unchanged implementation patches in the first four layers; eager loading preserves main’s composer button state and has a new individual review. A separate read-only combined merge-base review of `beefacf…4aadf77` completed with **No findings**. Its 25 focused source-context and preparation tests passed; no individual or combined review findings remain unresolved.

Validation completed:

- `sh scripts/build-assets.sh`.
- `go test -timeout=20m ./...` passed on the final rebased implementation, `4aadf77`, including the real Pi 1.3.0 pipeline, guardrail, scope, usage, legacy/current resume, compaction, OCC, storage, and browser suites.
- Gallery checks and focused browser lifecycle tests also passed before the rebase; the final individual eager-loading review passed all 127 focused browser/source-context cases after it.
- Extension and real-Pi source lifecycle checks passed for split-turn compaction, preservation of widget revisions, and source-role/freshness behavior.
- Exact OCC-layer extension tests passed all 30 cases, including failed first full-write retries; deterministic API regressions cover no-op restore races and conflict-before-quota ordering.
- Real browser checks passed deliberate-typing startup, Send reuse, and History → Continue → Send. The mock provider received no requests while composing or resuming; the first accepted request used prepared source and proceeded directly to a write. No console errors occurred.
- Node 22/Pi 1.3.0 Docker runtime installation/startup smoke passed. A complete application image build was attempted but stopped because image-layer operations were very slow; the complete image remains unverified.

## Compatibility and limits

Agent writes require revisions. Legacy owner/service API writes may omit them; those clients protect the server read/commit interval but cannot detect changes since their own earlier read. Revision identity follows immutable blob identity, including restores, rather than aggregate version history.

No model benchmarking or thinking summaries were added. T3's generated Guide tab was unavailable because review guides are disabled in Settings → Source Control; this document provides the stack guide instead.
