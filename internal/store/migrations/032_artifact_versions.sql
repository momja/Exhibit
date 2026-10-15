-- +goose Up
-- Artifact version history.
--
-- Until now an artifact was one mutable thing: an edit, a refetch or an agent
-- write overwrote the body blob in place and the previous document was gone.
-- A version is a point in the artifact's life that can be returned to, and it
-- is deliberately more than a diff of text: it carries the widget with it, and
-- the stored state the code had written by the time the next version replaced
-- it.
--
-- One row per version, never rewritten, with two exceptions that are each
-- written exactly once: state_json, when the version is superseded, and the
-- provenance of version 1 when an agent created the artifact.
--
-- The head is a version too. artifacts.source_blob_id and widget_blob_id are
-- kept as the head's denormalized pointers (the render path reads them), and
-- are only ever written together with the version row they mirror.
--
-- body_blob_id and widget_blob_id name blobs that are never overwritten: a
-- change writes a new blob, so an older version's bytes are exactly what they
-- were. A blob that did not change between versions (a widget, usually) is
-- named by both rows and stays alive while either does (blobqueue.go's
-- refcount, and the blob_references view below).
--
-- state_json is the state the version LEFT BEHIND: a snapshot of the owner's
-- artifact_state rows taken in the same transaction that created the next
-- version, so it is the state as it stood right before that change. NULL means
-- no snapshot has been taken, which is true of the head (its state is the live
-- rows) and of nothing else. A snapshot is an atomic JSON object rather than
-- rows because a restore returns the artifact to a coherent moment; the live
-- table keeps one row per key so concurrent writes to different keys don't
-- clobber each other, and an archive wants the opposite.
--
-- origin is one of initial, edit, agent, refetch, restore. message is a human
-- label (the prompt that caused an agent version, the source URL of a refetch,
-- "Restored v3"). session_id names the agent chat that wrote an agent version.
-- It is a plain string: the chat is a different table's business and may be
-- gone.
CREATE TABLE artifact_versions (
    artifact_id    TEXT     NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    seq            INTEGER  NOT NULL,
    origin         TEXT     NOT NULL DEFAULT 'edit',
    message        TEXT     NOT NULL DEFAULT '',
    session_id     TEXT     NOT NULL DEFAULT '',
    body_blob_id   TEXT     NOT NULL,
    widget_blob_id TEXT     NOT NULL DEFAULT '',
    state_json     TEXT,
    created_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (artifact_id, seq)
);

-- Every artifact has a version 1, so "the head is the highest seq" holds for
-- every artifact without any code path having to remember to create one. The
-- backfill gives the artifacts that already exist theirs...
INSERT INTO artifact_versions (artifact_id, seq, origin, body_blob_id, widget_blob_id, created_at)
SELECT id, 1, 'initial', source_blob_id, widget_blob_id, created_at FROM artifacts;

-- ...and the trigger gives every later one theirs, from the same columns. It
-- is a trigger, as tags_text and the share counts are, because the invariant
-- belongs to the schema: an INSERT that bypasses PutArtifact still gets it.
-- +goose StatementBegin
CREATE TRIGGER artifacts_initial_version
AFTER INSERT ON artifacts
BEGIN
    INSERT INTO artifact_versions (artifact_id, seq, origin, body_blob_id, widget_blob_id, created_at)
    VALUES (NEW.id, 1, 'initial', NEW.source_blob_id, NEW.widget_blob_id, NEW.created_at);
END;
-- +goose StatementEnd

-- Storage accounting learns about versions. blob_references is the extension
-- point 021 and 026 left for exactly this: replacing the view makes the usage
-- query, the recompute pass, the backfill and the unreferenced-size prune
-- count a body or widget that only an older version still names, with no code
-- change. The head's blobs appear twice (once through artifacts, once through
-- its version row); every reader takes DISTINCT blob_id per owner, so they are
-- charged once.
DROP VIEW blob_references;

CREATE VIEW blob_references AS
    SELECT source_blob_id AS blob_id, owner_id FROM artifacts WHERE source_blob_id != ''
    UNION ALL
    SELECT widget_blob_id AS blob_id, owner_id FROM artifacts WHERE widget_blob_id != ''
    UNION ALL
    SELECT aa.blob_id AS blob_id, a.owner_id AS owner_id
      FROM artifact_assets aa
      JOIN artifacts a ON a.id = aa.artifact_id
     WHERE aa.blob_id != ''
    UNION ALL
    SELECT v.body_blob_id AS blob_id, a.owner_id AS owner_id
      FROM artifact_versions v
      JOIN artifacts a ON a.id = v.artifact_id
     WHERE v.body_blob_id != ''
    UNION ALL
    SELECT v.widget_blob_id AS blob_id, a.owner_id AS owner_id
      FROM artifact_versions v
      JOIN artifacts a ON a.id = v.artifact_id
     WHERE v.widget_blob_id != '';

-- +goose Down
DROP VIEW blob_references;

CREATE VIEW blob_references AS
    SELECT source_blob_id AS blob_id, owner_id FROM artifacts WHERE source_blob_id != ''
    UNION ALL
    SELECT widget_blob_id AS blob_id, owner_id FROM artifacts WHERE widget_blob_id != ''
    UNION ALL
    SELECT aa.blob_id AS blob_id, a.owner_id AS owner_id
      FROM artifact_assets aa
      JOIN artifacts a ON a.id = aa.artifact_id
     WHERE aa.blob_id != '';

DROP TRIGGER artifacts_initial_version;
DROP TABLE artifact_versions;
