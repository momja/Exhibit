-- +goose Up
-- Tag names are unique per owner. Fold any pre-existing duplicates into the
-- first-created tag of each (owner, name) before adding the index: re-point
-- attachments at the survivor, then drop the duplicates (attachments that
-- could not be re-pointed follow them via ON DELETE CASCADE).
UPDATE OR IGNORE artifact_tags SET tag_id = (
    SELECT t2.id FROM tags t2
    JOIN tags t1 ON t1.id = artifact_tags.tag_id
    WHERE t2.owner_id = t1.owner_id AND t2.name = t1.name
    ORDER BY t2.rowid LIMIT 1
);
DELETE FROM tags WHERE rowid NOT IN (
    SELECT MIN(rowid) FROM tags GROUP BY owner_id, name
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_owner_name ON tags(owner_id, name);

-- +goose Down
DROP INDEX IF EXISTS idx_tags_owner_name;
