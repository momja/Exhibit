# Exhibit implementation simplicity review

Workspace /home/maxomdal/.t3/worktrees/Exhibit/feature-Exh-zrjo-ui-council; branch feature/Exh-zrjo/ui-council.
PINNED base ab0135c902559d5912deec420947d6ab3cbdbcf6, PINNED review head 26dae10d.
Full diff command: git diff ab0135c902559d5912deec420947d6ab3cbdbcf6...26dae10d
Application-code command: git diff ab0135c902559d5912deec420947d6ab3cbdbcf6...26dae10d -- internal web e2e cmd scripts Makefile go.mod go.sum
Standards-document delta: git diff ab0135c902559d5912deec420947d6ab3cbdbcf6...26dae10d -- docs/ui-components.md docs/technical_stack.md docs/mobile-readability.md
Complete commit list: /tmp/exhibit-zrjo-simplicity-commits.txt (read it).
Base resolves, is merge-base, and diff is nonempty.77 runtime/test/build files,
3269additions/776deletions; generated review records/screenshots are evidence,
not application implementation. Review actual application diff and relevant docs.
Use pinned git objects if inspecting anything changed while you work.

Latest USER: Good. Now review your code for simplicity. Make sure to use htmx
swaps and our JS library where it makes sense, check for good hygiene with our
components and reusability. Ask subagents to review the diff.

Scope is the cumulative OUR implementation delta atop PR273 base, including
prior gallery/editor changes and the latest tag work. PR273 gallery redesign is
an authorized baseline, not our new implementation. Earlier full UI work was
authorized; do not call it scope creep solely because later visual scoring was
narrowed to Tags. Need preserve the approved appearance and current behaviors.
This is CODEreview, not another subjective visual scoring round.

Spec sources: .tickets/Exh-zrjo.md (acceptance and history), original docs/reviews/
Exh-zrjo/tags-ten-brief.md, docs/product_requirement_doc.md, and latest user text
above. Gallery tabs/access/conversation architecture from PR273 must remain.
Prior scoped tag visual target was completed on f58de10b; code review followup
must not rewrite historical model reports or claim new visual ratings.

Standards sources (no CONTRIBUTING/STANDARDS/STYLE files found): AGENTS.md,
CLAUDE.md, docs/ui-components.md, docs/technical_stack.md (especially§9),
docs/architecture.md, docs/build_assets.md, docs/mobile-readability.md and relevant
docs/security.md. Read pertinent sections; current and baseline standards if our
diff changes documentation. Key rules: Go templates own markup/data; htmx swaps
server regions after mutations; native page ES modules import dependencies;
Stimulus owns reusable behavior/lifecycle; compose shared modalFor/editor-session/
apiFetch/outside-press APIs. Self-host Phosphor. Deep/simple modules, precise names.
Avoid speculative abstraction or line-count-only rewrites.

Review only. NO source edits/builds/tests/commits/persisted fixture mutations.
You may write only your uniquely assigned /tmp report. Do not read another
reviewer's report. Return concise actionable findings with file:line, quote the
standard/spec as relevant, hard violations vs heuristic suggestions, severity,
concrete simpler reuse. Under400words per report. Focus on real diff issues and
regressions, not hypothetical style preferences already enforced by tooling.
