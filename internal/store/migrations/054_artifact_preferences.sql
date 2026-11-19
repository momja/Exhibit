-- +goose Up
CREATE TABLE artifact_preferences (
    -- No-login instances use owner 1 before any users row exists, just as
    -- artifact_state does. Real account deletion still erases viewer rows.
    viewer_id INTEGER NOT NULL,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    starred INTEGER NOT NULL DEFAULT 0 CHECK(starred IN (0,1)),
    last_opened_at TEXT,
    PRIMARY KEY (viewer_id, artifact_id)
);
CREATE INDEX artifact_preferences_starred ON artifact_preferences(viewer_id, starred, last_opened_at DESC, artifact_id);

-- +goose StatementBegin
CREATE TRIGGER artifact_preferences_user_delete AFTER DELETE ON users
BEGIN
    DELETE FROM artifact_preferences WHERE viewer_id = OLD.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER artifact_preferences_user_delete;
DROP TABLE artifact_preferences;
