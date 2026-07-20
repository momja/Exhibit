---
id: av-fvci
status: open
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


## Notes

**2026-07-20T02:00:47Z**

Implementation PR #54 (local-convention path A: TICKETS.md vocabulary + tk-ready-complexity.sh + backfill) was closed by @momja: 'I don't see a path forward with the existing implementation of ticket. They haven't released a version that supports plugins yet.' Returning ticket to open; likely blocked on upstream (see wedow/ticket#66, the first-class-field proposal filed as part of the attempt). Branch chore/av-fvci/complexity-field retains the work if revisited.

**2026-07-20T03:47:56Z**

Tag-based option (raised 2026-07-20): express complexity as a reserved tag
prefix — cx-xs / cx-s / cx-m / cx-l / cx-xl — instead of a body section.

Why this beats path A: tags are a real frontmatter field tk already supports at
creation (--tags) and can filter on (tk ls -T cx-l), so complexity becomes
queryable *today* with no wrapper script and no upstream change. That was the
exact blocker that closed PR #54 — the local convention was unqueryable, and
the queryable version needed a plugin/field tk hasn't shipped. Tags route around
both.

What it doesn't give us, and should be accepted knowingly:
- Nothing enforces exactly one cx-* tag per ticket; a mis-tagged or double-tagged
  ticket is silently wrong. Convention + a lint script are the only guards.
- Tags are an unordered flat namespace, so "everything at least M" is not
  expressible as one query — it's a union of -T cx-m, -T cx-l, -T cx-xl.
- It overloads one field with two taxonomies (topic tags like `security` next to
  sizing tags), which is why the cx- prefix matters: it keeps the sizing
  vocabulary greppable and visually distinct from topic tags.

If adopted, this ticket's acceptance criteria change shape: define the cx-*
vocabulary in TICKETS.md (agent-tier mapping unchanged), backfill obvious cases,
and keep wedow/ticket#66 open as the eventual first-class-field cleanup rather
than a blocker.
