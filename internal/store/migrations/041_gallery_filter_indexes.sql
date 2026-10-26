-- +goose Up
CREATE INDEX artifact_tags_by_tag ON artifact_tags(tag_id, artifact_id);
CREATE INDEX artifact_collections_by_collection ON artifact_collections(collection_id, artifact_id);

-- +goose Down
DROP INDEX artifact_collections_by_collection;
DROP INDEX artifact_tags_by_tag;
