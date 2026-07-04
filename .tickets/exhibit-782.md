---
id: exhibit-782
status: closed
deps: []
links: []
created: 2026-07-02T05:41:49Z
type: chore
priority: 2
assignee: Max Omdal
---
# Document parallel-worktree beads reconciliation in CLAUDE.md

Parallel worktree agents fork beads state — each worktree runs its own Dolt server+DB (server mode), so agent bd claim/close is invisible on main, and a fresh worktree can bootstrap a wrong dolt_database name whose committed metadata.json 'fix' must not merge to main. Hit during the 2026-07-01 exhibit-oxm/ep9/tww.1.1 fan-out. Add a 'Parallel Worktree Agents & Beads' subsection under Version Control in CLAUDE.md: cross-worktree bead status is not shared; reconcile on canonical main via bd dolt pull + conflict resolve + bd dolt push (verify issue count); never merge .beads/metadata.json or a worktree's issues.jsonl to main. Cross-ref memories gotcha-beads-worktree-agent-divergence and gotcha-supacode-worktree-agent-bare-claude-tab.


