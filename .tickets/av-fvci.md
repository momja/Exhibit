---
id: av-fvci
status: in_progress
deps: []
links: []
created: 2026-07-08T07:29:36Z
type: chore
priority: 3
assignee: Max Omdal
tags: [tickets, tooling, agents]
---
# Add 'complexity' field to ticket tracking for agent-sizing

Today tickets carry a numeric 'priority' (urgency) but nothing describes how hard a ticket is or what caliber of agent it needs. Add a 'complexity' indicator — analogous to dev story points, but oriented toward *what kind of agent can complete this work* (e.g. trivial copy edit vs. nuanced cross-system refactor requiring context-load and judgement). Goal: make 'ready'/'backlog' triage surface which tickets a lightweight agent can knock out vs. which need a heavier, context-aware one.

## Design

tk is an external Homebrew package (ticket 0.3.2 at /opt/homebrew/Cellar/ticket) — its frontmatter keys (id, status, deps, links, created, type, priority, assignee, parent, tags) are fixed. Two paths:

A. LOCAL CONVENTION (no upstream dep, ships now): adopt a 'complexity' line in the ticket body or a custom frontmatter-ish section that tk ignores. Concrete options:
   - A '## Complexity' section in the body with a controlled vocabulary, e.g. one of: XS (trivial/mechanical), S (single-file logic), M (multi-file, clear shape), L (cross-system, needs architecture context), XL (open-ended design). Plus an optional 'agent: lightweight | standard | heavy' hint derived from the same scale.
   - Pro: works today, no tool change. Con: tk ls/ready/query can't filter on it; it's documentation, not queryable state. A small wrapper script (grep/.tickets) can parse it if needed.
B. UPSTREAM FIELD (proper, queryable): contribute a 'complexity' frontmatter field to the 'ticket' project (https://github.com/wedow/ticket, on the wedow/tools Homebrew tap) so 'tk ls --complexity=XS' / 'tk ready' can filter. Pro: queryable, first-class. Con: depends on an external project, slower to land, version coupling.

Recommendation: ship path A as the convention now (this chore = write the convention into AGENTS.md or a TICKETS.md, backfill a '## Complexity' line on new tickets, add a one-line wrapper flag to 'tk ready' if filtering is wanted), and open an upstream issue for path B in parallel. Keep the vocabulary short and agent-oriented (not time-oriented) so it stays about *who/what can do it*, not *how long* — that's what distinguishes it from priority and from story points.

Name the field 'complexity' (not 'size'/'points') to avoid story-points baggage and to keep the agent-sizing intent explicit.

## Acceptance Criteria

- A 'complexity' vocabulary is defined in repo docs (AGENTS.md 'Ticket System' section or a new TICKETS.md), with each level mapping to the kind of agent/work it implies (XS/S/M/L/XL + agent-tier hint).
- New tickets carry a '## Complexity' entry (or equivalent agreed slot) at creation; the convention is referenced from AGENTS.md so agents follow it.
- Existing open tickets are backfilled where the level is obvious; the rest are filled in on next touch (no big-bang audit required).
- If a filtering wrapper is in scope, a small script/alias surfaces 'tk ready' grouped or filtered by complexity.
- An upstream issue is filed at https://github.com/wedow/ticket proposing a first-class 'complexity' frontmatter field (link recorded as a note on this ticket).


## Complexity

M

## Notes

**2026-07-18T15:48:56Z**

Upstream issue filed proposing a first-class complexity frontmatter field: https://github.com/wedow/ticket/issues/66
