-- +goose Up
-- Artifact inference (Exh-zue3): an artifact may call an AI model through the
-- host frame, and the person viewing it pays.
--
-- Version numbering: read migration_repair.go's header and technical_stack.md
-- §3 before adding another. 056 was the next number above every version in the
-- ledger when this was written; rebase and renumber if main has moved past it.
--
-- 1. inference_approved is the artifact-level first-use approval, the seventh
--    store.ApprovalColumns entry. Only the owner sets it, through the ordinary
--    first-use prompt or the edit page's Permissions panel.
ALTER TABLE artifacts ADD COLUMN inference_approved INTEGER NOT NULL DEFAULT 0;

-- 2. A named recipient spends their own allowance (or key), so each of them
--    also consents once per artifact, in the shape of artifact_relay_grants.
--    The owner's own consent is inference_approved itself and is never a row
--    here. No users FK: a no-login owner has no users row; the trigger below
--    removes a deleted account's rows instead.
CREATE TABLE artifact_inference_consents (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    viewer_id   INTEGER NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (artifact_id, viewer_id)
) WITHOUT ROWID;
CREATE INDEX artifact_inference_consents_viewer ON artifact_inference_consents(viewer_id);
-- +goose StatementBegin
CREATE TRIGGER artifact_inference_consents_account_delete AFTER DELETE ON users BEGIN
    DELETE FROM artifact_inference_consents WHERE viewer_id = OLD.id;
END;
-- +goose StatementEnd

-- 3. The ledger. Inference spend lands in agent_usage rather than a second
--    table so the shared spend ceilings keep reading one sum (Exh-hwjg), with
--    a new source and two nullable columns that let it be reported apart:
--    artifact_id (whose code made the call; deliberately no FK — accounting
--    outlives the artifact) and kind ('complete' | 'decide'). owner_id keeps
--    its meaning of "whose budget this row is": for an inference row that is
--    the paying viewer, never the artifact's owner.
--
--    SQLite cannot widen a CHECK in place, so the table is rebuilt. Column
--    order is preserved and ids are copied, so nothing that read a row id sees
--    a change.
CREATE TABLE agent_usage_056 (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id           INTEGER NOT NULL,
    session_id         TEXT NOT NULL,
    provider           TEXT NOT NULL,
    model              TEXT NOT NULL DEFAULT '',
    paid_by            TEXT NOT NULL CHECK (paid_by IN ('platform', 'user')),
    source             TEXT NOT NULL CHECK (source IN ('model', 'tool', 'compaction', 'guardrail', 'artifact_inference')),
    input_tokens       INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens      INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
    cache_write_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
    cost_micros        INTEGER NOT NULL DEFAULT 0 CHECK (cost_micros >= 0),
    recorded_at        TEXT NOT NULL DEFAULT (datetime('now')),
    artifact_id        TEXT,
    kind               TEXT CHECK (kind IS NULL OR kind IN ('complete', 'decide')),
    -- provider = the cost came from the provider's own report; catalog = Pi's
    -- price table; none = no usage was reported (unknown, not zero, spend).
    cost_source        TEXT NOT NULL DEFAULT 'catalog' CHECK (cost_source IN ('provider', 'catalog', 'none'))
);
INSERT INTO agent_usage_056 (id, owner_id, session_id, provider, model, paid_by, source,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_micros, recorded_at)
SELECT id, owner_id, session_id, provider, model, paid_by, source,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_micros, recorded_at
FROM agent_usage;
DROP TABLE agent_usage;
ALTER TABLE agent_usage_056 RENAME TO agent_usage;
CREATE INDEX agent_usage_owner_period ON agent_usage(owner_id, recorded_at);
CREATE INDEX agent_usage_session ON agent_usage(session_id);
CREATE INDEX agent_usage_paid_period ON agent_usage(paid_by, recorded_at);
CREATE INDEX agent_usage_inference ON agent_usage(owner_id, artifact_id, recorded_at) WHERE artifact_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS agent_usage_inference;
CREATE TABLE agent_usage_055 (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id           INTEGER NOT NULL,
    session_id         TEXT NOT NULL,
    provider           TEXT NOT NULL,
    model              TEXT NOT NULL DEFAULT '',
    paid_by            TEXT NOT NULL CHECK (paid_by IN ('platform', 'user')),
    source             TEXT NOT NULL CHECK (source IN ('model', 'tool', 'compaction', 'guardrail')),
    input_tokens       INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens      INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
    cache_write_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
    cost_micros        INTEGER NOT NULL DEFAULT 0 CHECK (cost_micros >= 0),
    recorded_at        TEXT NOT NULL DEFAULT (datetime('now'))
);
-- Inference rows cannot be represented before 056; they are dropped, which is
-- what rolling the feature back means.
INSERT INTO agent_usage_055 SELECT id, owner_id, session_id, provider, model, paid_by, source,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_micros, recorded_at
FROM agent_usage WHERE source != 'artifact_inference';
DROP TABLE agent_usage;
ALTER TABLE agent_usage_055 RENAME TO agent_usage;
CREATE INDEX agent_usage_owner_period ON agent_usage(owner_id, recorded_at);
CREATE INDEX agent_usage_session ON agent_usage(session_id);
CREATE INDEX agent_usage_paid_period ON agent_usage(paid_by, recorded_at);
DROP TRIGGER IF EXISTS artifact_inference_consents_account_delete;
DROP TABLE IF EXISTS artifact_inference_consents;
ALTER TABLE artifacts DROP COLUMN inference_approved;
