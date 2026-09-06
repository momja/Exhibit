---
id: av-lrae
status: in_progress
deps: []
links: [av-20xv]
created: 2026-09-06T01:32:48Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-7k7b
tags: [sharing, store, schema, security]
---
# Grant schema and the non-owner read accessor

Slice 1 of sharing v1, and everything else depends on it. Two halves: the schema
that records a grant, and the read path that honours one without widening what
an owner-scoped query means.

Design is settled on av-7k7b; this ticket builds it. Read that ticket's
2026-09-05 notes before starting rather than re-deriving anything here.

## Schema

    ALTER TABLE shares ADD COLUMN recipient_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
    ALTER TABLE artifacts ADD COLUMN share_state_mode TEXT NOT NULL DEFAULT 'own';
    ALTER TABLE shares DROP COLUMN public;
    CREATE UNIQUE INDEX shares_artifact_recipient ON shares(artifact_id, recipient_id);
    CREATE UNIQUE INDEX shares_one_anonymous_link ON shares(artifact_id) WHERE recipient_id IS NULL;

- recipient_id NULL is the anonymous link; set, it is a grant to that user.
- The PARTIAL index is what enforces one anonymous link per artifact. The
  composite index does not: SQLite treats every NULL in a unique index as
  distinct, so it constrains grants and says nothing about the link. Both are
  needed.
- Dropping `public` closes av-20xv. It becomes exactly `recipient_id IS NULL`,
  so keeping it is two columns encoding one fact with nothing stopping them
  disagreeing.
- share_state_mode goes on ARTIFACTS, not on the share row. One answer per
  artifact; per-grant would admit one recipient on their own rows while another
  writes the owner's.

Migration numbering: the ledger stands at 027 SQL plus Go migrations 8, 12, 13
and 23, so 028 at time of writing. Re-check before writing the file and rebase
off main before opening a PR — technical_stack.md section 3 catalogues four
outages from this.

## The read accessor

ownsArtifact and the owner-scoped EXISTS subquery are NOT touched. Reaching a
shared artifact goes through a separate, deny-by-default accessor naming only
the routes a recipient may use.

The rejected alternative was widening the existing predicate to 'owner, or named
on a live grant'. One change and every query picks it up — DELETE, the body
rewrite, share revocation — so a recipient would inherit the ability to destroy
the artifact, and it would fail silently. Deny-by-default is the shape
agentSubResources and public mode's route allowlist already use.

Two owner concepts now exist per request and must not be conflated: the artifact
OWNER (authorizes, may mutate) and the VIEWER (may read, and may write state).
That is av-q0ub's split reaching the artifact itself. Name the accessor for what
it grants, not for who calls it.

## Acceptance Criteria

1. Migration adds recipient_id, artifacts.share_state_mode and both indexes, and
   drops shares.public. A database left at the previous release upgrades and
   starts (the existing end-to-end upgrade test covers the shape).
2. Two rows with recipient_id NULL for one artifact are refused by the database,
   not by a handler.
3. Two grants of one artifact to one user are refused by the database.
4. ownsArtifact is unchanged. A grep of its call sites shows no route gained
   non-owner reach through it.
5. A recipient can read the artifact through the new accessor and CANNOT reach
   DELETE, PATCH, the body rewrite, the origins routes, the capability approval
   columns, share creation or revocation. Each refusal is 404, never 403.
6. A user with no grant is indistinguishable from a nonexistent artifact.
7. Deleting a recipient's account removes their grants (FK cascade) and their
   state rows (migration 014's users trigger). Test both.
8. Deleting the artifact removes its grants (existing FK cascade).

