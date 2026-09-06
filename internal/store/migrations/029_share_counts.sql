-- +goose Up
-- av-6xjd: make an artifact's sharing state visible from the gallery.
--
-- The failure this exists for is the share made eight months ago that nobody
-- has thought about since — and a grant now carries live state, so forgetting
-- one costs more than it used to. You cannot audit what you cannot list, and
-- ListArtifacts carried no share data at all: no join, no count, so the
-- library could not show a badge even if the template wanted one.
--
-- Two denormalized columns rather than a join on every gallery render, which
-- is the trade artifacts.tags_text already made (migration 010, av-b6o9): the
-- index page reads a hundred artifacts and asks one question of each, and the
-- answer changes only when a share row is written. Triggers keep them current,
-- so nothing in the write path has to remember — the same property that makes
-- tags_text correct for a caller who never heard of it.
--
-- Version numbering: 8, 12 and 23 are Go migrations with no file here, and the
-- highest .sql is 028. A migration must sit above *every* version that can
-- already be in a ledger, from either source (technical_stack.md §3 catalogues
-- four outages from getting this wrong); migration_order_test.go walks both
-- rules.

-- How many accounts hold a grant on this artifact, and whether its one
-- anonymous link exists. Two columns because they are two different facts
-- about who may open the artifact — named people, and anybody holding a URL —
-- and the badge names the stronger of the two.
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

-- +goose Down
DROP TRIGGER IF EXISTS shares_counts_sync_update;
DROP TRIGGER IF EXISTS shares_counts_sync_delete;
DROP TRIGGER IF EXISTS shares_counts_sync_insert;
ALTER TABLE artifacts DROP COLUMN share_link;
ALTER TABLE artifacts DROP COLUMN share_grant_count;
