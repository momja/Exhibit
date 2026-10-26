-- +goose Up
-- Exh-ituu: the standalone-worker authority layer (035) was de-scoped before
-- anything constructed it, so these tables were never written. 035 stays, since
-- shipped databases have applied it; this removes what it created.
DROP TABLE IF EXISTS agent_usage_receipts;
DROP TABLE IF EXISTS agent_supervisor_credentials;
DROP TABLE IF EXISTS agent_executions;
DROP TABLE IF EXISTS agent_conversations;

-- +goose Down
-- Intentionally empty: 035's Down already drops these tables, and recreating
-- them here would revive a schema nothing reads.
