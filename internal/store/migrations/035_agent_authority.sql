-- +goose Up
-- Durable authority is separate from transcript contents and the existing usage
-- ledger. Accounting and attempt identities survive artifact/account deletion.
CREATE TABLE IF NOT EXISTS agent_conversations (
 id TEXT PRIMARY KEY,
 owner_id INTEGER NOT NULL,
 artifact_id TEXT NOT NULL DEFAULT '',
 generation INTEGER NOT NULL DEFAULT 0,
 snapshot_revision INTEGER NOT NULL DEFAULT 0,
 checkpoint_hash TEXT NOT NULL DEFAULT '',
 checkpoint_kind TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS agent_executions (
 execution_id TEXT PRIMARY KEY,
 conversation_id TEXT NOT NULL REFERENCES agent_conversations(id),
 worker_id TEXT NOT NULL,
 generation INTEGER NOT NULL CHECK(generation > 0),

 -- Last version observed by the core for this attempt, initialized at reserve
 -- or create-mode bind and advanced only by a core-verified agent version.
 artifact_version_seq INTEGER NOT NULL DEFAULT 0,
 status TEXT NOT NULL CHECK(status IN ('reserved','active','closing','closed','failed','interrupted')),
 lease_until INTEGER NOT NULL,
 UNIQUE(conversation_id,generation)
);
CREATE UNIQUE INDEX IF NOT EXISTS agent_execution_writer ON agent_executions(conversation_id)
 WHERE status IN ('reserved','active','closing');
CREATE INDEX IF NOT EXISTS agent_execution_capacity ON agent_executions(status,lease_until);
CREATE TABLE IF NOT EXISTS agent_supervisor_credentials (
 execution_id TEXT PRIMARY KEY REFERENCES agent_executions(execution_id),
 token_hash BLOB NOT NULL UNIQUE,
 paid_by TEXT NOT NULL CHECK(paid_by IN ('platform','user')),
 expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_usage_receipts (
 execution_id TEXT NOT NULL REFERENCES agent_executions(execution_id),
 event_id TEXT NOT NULL,
 payload_hash TEXT NOT NULL,
 PRIMARY KEY(execution_id,event_id)
);
-- +goose Down
DROP TABLE IF EXISTS agent_usage_receipts;
DROP TABLE IF EXISTS agent_supervisor_credentials;
DROP TABLE IF EXISTS agent_executions;
DROP TABLE IF EXISTS agent_conversations;
