-- +goose Up
-- Network origin decisions move from the artifacts.network_allowlist JSON array
-- to a relational child table (exhibit-x87), matching the artifact_state /
-- artifact_tags idiom. The primary key enforces one decision per (artifact,
-- origin); ON DELETE CASCADE drops the rows with the artifact.
--
-- decision='allow' rows are the source of truth for the render CSP.
-- decision='block' rows are "do not re-prompt" markers for the runtime
-- permission prompt (exhibit-fr7) and NEVER affect the CSP.
CREATE TABLE IF NOT EXISTS artifact_network_origins (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    origin TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('allow', 'block')),
    source TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (artifact_id, origin)
);

-- Backfill the existing JSON arrays as allow decisions. Pre-v1 data volume is
-- negligible; a malformed array is skipped rather than failing the migration.
INSERT OR IGNORE INTO artifact_network_origins (artifact_id, origin, decision, source)
SELECT a.id, j.value, 'allow', 'legacy'
FROM artifacts a, json_each(a.network_allowlist) j
WHERE json_valid(a.network_allowlist) AND j.value <> '';

ALTER TABLE artifacts DROP COLUMN network_allowlist;

-- +goose Down
ALTER TABLE artifacts ADD COLUMN network_allowlist TEXT NOT NULL DEFAULT '[]';
DROP TABLE IF EXISTS artifact_network_origins;
