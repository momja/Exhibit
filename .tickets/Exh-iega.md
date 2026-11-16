---
id: Exh-iega
status: in_progress
deps: []
links: []
created: 2026-11-16T04:06:35Z
type: chore
priority: 2
assignee: Max Omdal
---
# Allow explicitly authorized agent PR merges after independent review

Replace the blanket AGENTS.md merge prohibition with a narrow exception for tasks explicitly authorized by the user for autonomous PR completion. Keep the detailed T3 operating policy outside the repository during the pilot. This task creates the policy-change PR only; it does not authorize merging that PR.

## Design

Keep main-worktree and direct-push restrictions. Allow GitHub PR merges only with explicit task authorization, an independent agent PASS tied to the final head commit, required checks passing, and GitHub protections satisfied. Authorize the independent review delegation needed for that gate only for tasks with autonomous merge authorization. Escalate unresolved blocking findings, scope changes, or missing qualification evidence rather than bypassing protections.

## Acceptance Criteria

AGENTS.md retains isolated branch/worktree development and no direct push/local merge to main; GitHub merge exception requires explicit user task authorization plus final-head independent PASS and passing required checks; reviewer is separate from author and blocking findings must be resolved; task-level authorization includes review delegation; no new workflow docs, templates, or ticket eligibility mechanism; ticket is committed on main then closed on implementation branch; PR is opened for human review without merging.

