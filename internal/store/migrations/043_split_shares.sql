-- +goose Up
-- Exh-grj9: links are capability URLs; grants are (artifact, account) facts.
CREATE TABLE share_links (
    id TEXT PRIMARY KEY,
    artifact_id TEXT NOT NULL UNIQUE REFERENCES artifacts(id) ON DELETE CASCADE
);
CREATE TABLE share_grants (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (artifact_id, recipient_id)
) WITHOUT ROWID;
CREATE INDEX share_grants_by_recipient ON share_grants(recipient_id);

-- Keep link identities: /s/:id URLs and cached OG snapshots use them.
INSERT INTO share_links (id, artifact_id)
    SELECT id, artifact_id FROM shares WHERE recipient_id IS NULL;
-- Legacy shares have no timestamp. Preserve their insertion (rowid) order
-- with synthetic millisecond timestamps, safely before any newly created grant.
INSERT INTO share_grants (artifact_id, recipient_id, created_at)
    SELECT artifact_id, recipient_id,
           strftime('%Y-%m-%dT%H:%M:%fZ', '2000-01-01',
                    (row_number() OVER (ORDER BY rowid) / 1000.0) || ' seconds')
      FROM shares WHERE recipient_id IS NOT NULL;

-- DROP TABLE removes shares' indexes and shares_counts_sync_* triggers.
-- DROP COLUMN requires that no surviving index, view, trigger or constraint
-- names either column (028). Only those three dropped triggers did so.
DROP TABLE shares;
ALTER TABLE artifacts DROP COLUMN share_link;
ALTER TABLE artifacts DROP COLUMN share_grant_count;

-- +goose Down
-- Grant ids and link/grant interleaving were discarded by Up and cannot return.
-- Regenerate grant ids; link ids, recipients and relative grant order survive.
CREATE TABLE shares (
    id TEXT PRIMARY KEY,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    recipient_id INTEGER REFERENCES users(id) ON DELETE CASCADE
);
INSERT INTO shares (id, artifact_id, recipient_id)
    SELECT id, artifact_id, NULL FROM share_links;
INSERT INTO shares (id, artifact_id, recipient_id)
    SELECT lower(hex(randomblob(16))), artifact_id, recipient_id FROM share_grants
     ORDER BY artifact_id, created_at, recipient_id;
CREATE UNIQUE INDEX shares_artifact_recipient ON shares(artifact_id, recipient_id);
CREATE UNIQUE INDEX shares_one_anonymous_link ON shares(artifact_id) WHERE recipient_id IS NULL;
DROP TABLE share_grants;
DROP TABLE share_links;

-- Restore 029's columns, backfill and triggers for the previous application.
ALTER TABLE artifacts ADD COLUMN share_grant_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE artifacts ADD COLUMN share_link INTEGER NOT NULL DEFAULT 0;

UPDATE artifacts SET
    share_grant_count = (SELECT COUNT(*) FROM shares
                          WHERE shares.artifact_id = artifacts.id
                            AND shares.recipient_id IS NOT NULL),
    share_link = (SELECT EXISTS(SELECT 1 FROM shares
                                 WHERE shares.artifact_id = artifacts.id
                                   AND shares.recipient_id IS NULL));

-- Each trigger recomputes both values from the shares table rather than
-- incrementing or decrementing. A rollup cannot drift; a counter can, and the
-- one thing worse than no badge is a badge that says "private" about an
-- artifact three people can open. It is also what makes the cascades correct
-- for free: deleting an artifact, or the account a grant names, removes the
-- rows and these triggers follow, with no delete path needing to know.
--
-- Updating artifacts fires artifacts_fts_update, which re-indexes the row. That
-- is the same cost tags_text's triggers already pay and it is bounded by how
-- often somebody shares something. Note what is deliberately NOT touched:
-- updated_at. Granting access is not an edit of the artifact, and moving it to
-- the top of a gallery sorted by recency would say otherwise.

-- +goose StatementBegin
CREATE TRIGGER shares_counts_sync_insert AFTER INSERT ON shares BEGIN
    UPDATE artifacts SET
        share_grant_count = (SELECT COUNT(*) FROM shares
                              WHERE artifact_id = new.artifact_id
                                AND recipient_id IS NOT NULL),
        share_link = (SELECT EXISTS(SELECT 1 FROM shares
                                     WHERE artifact_id = new.artifact_id
                                       AND recipient_id IS NULL))
    WHERE id = new.artifact_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER shares_counts_sync_delete AFTER DELETE ON shares BEGIN
    UPDATE artifacts SET
        share_grant_count = (SELECT COUNT(*) FROM shares
                              WHERE artifact_id = old.artifact_id
                                AND recipient_id IS NOT NULL),
        share_link = (SELECT EXISTS(SELECT 1 FROM shares
                                     WHERE artifact_id = old.artifact_id
                                       AND recipient_id IS NULL))
    WHERE id = old.artifact_id;
END;
-- +goose StatementEnd

-- Nothing updates a shares row today: a share is minted and revoked, never
-- edited. The trigger is here anyway, and it re-rolls BOTH the old and the new
-- artifact, because the alternative is a rollup that silently stops being true
-- the first time somebody adds an UPDATE — which is precisely the class of
-- drift the recompute above was chosen to rule out.
-- +goose StatementBegin
CREATE TRIGGER shares_counts_sync_update AFTER UPDATE ON shares BEGIN
    UPDATE artifacts SET
        share_grant_count = (SELECT COUNT(*) FROM shares
                              WHERE artifact_id = artifacts.id
                                AND recipient_id IS NOT NULL),
        share_link = (SELECT EXISTS(SELECT 1 FROM shares
                                     WHERE artifact_id = artifacts.id
                                       AND recipient_id IS NULL))
    WHERE id IN (old.artifact_id, new.artifact_id);
END;
-- +goose StatementEnd
