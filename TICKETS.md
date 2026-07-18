# Ticketing Procedure for Agents
## Adding new tickets
- If you are asked to add a ticket, commit it directly to main. It _must_ be committed to main, to ensure it is not lost.

## Working on a ticket
1. Before creating a feature/bug/chore branch, update the ticket status to "In Progress" in the ticketing system on the main branch.
2. Commit the ticket update to main branch. This is the _only_ commit that should be made to the main branch and it ensures other agents don't pick up the work.
3. Create a new branch for the feature/bug/chore and continue development.

## Complexity ratings (agent sizing)

Tickets carry a numeric `priority` (urgency) but nothing about _how hard_ the
work is. Every ticket should also carry a **complexity** rating so triage can
match tickets to the caliber of agent able to complete them. Complexity is
about _what kind of agent can do the work_, never about how long it takes —
that is what distinguishes it from both `priority` and from story points.

### Convention

Add a `## Complexity` section to the ticket body whose first line is exactly
one of the levels below, optionally followed by an em-dash note:

```markdown
## Complexity

M — multi-file, but the shape is clear from the docs
```

`tk` frontmatter keys are fixed (upstream tool), so this is a repo-local body
convention that `tk` safely ignores.

### Levels

| Level | Meaning | Agent tier |
| ----- | ------- | ---------- |
| `XS` | Trivial, mechanical: typo, config line, file removal | lightweight |
| `S` | Single-file logic; local reasoning only | lightweight |
| `M` | Multi-file change with a clear shape; moderate context load | standard |
| `L` | Cross-system change; needs architecture context and judgement | heavy |
| `XL` | Open-ended design or epic-scale; ambiguous requirements | heavy + human; consider splitting first |

### Workflow

- Rate every new ticket at creation — agents filing tickets include the
  `## Complexity` section.
- Backfill on touch: when you pick up an unrated ticket, add your rating as
  part of starting it. No big-bang audit.
- `scripts/tk-ready-complexity.sh` groups `tk ready` output by rating, and
  filters (e.g. `scripts/tk-ready-complexity.sh XS S` lists only
  lightweight-agent-ready tickets).
