-- +goose Up
-- Agent token metering (av-2yws): what the agent spent, as rows one recorded
-- usage event at a time.
--
-- Version numbering: read migration_repair.go's header and technical_stack.md
-- §3 before adding another — a number reused is applied once and forever, and
-- a number below the ledger's high-water mark stops the instance from
-- starting at all. 030 was the next free number when this was written; it is
-- NOT a reservation. Rebase off main and renumber above every version in the
-- ledger before this merges.
--
-- One row per recorded usage event rather than one running total per session:
-- the finest granularity available is the point of this table (av-2yws).
-- Per-session rows aggregate to per-owner totals; the reverse is not
-- recoverable. Rows land as usage arrives — per assistant message, per tool
-- result that carries usage, per compaction — so a session that is aborted,
-- idle-reaped, or killed mid-turn still attributes what it spent.
--
-- provider and model stay on the row even though platform mode strips them
-- from every user-visible surface (av-siqf): that is a presentation decision,
-- and cost per token differs by model, so a total without it cannot become
-- money. paid_by separates the two money sources — 'platform' is the
-- instance's credential and guardrail screening, 'user' is a BYO-key session
-- spending its owner's own tokens (av-99f4: BYO-key sessions are never
-- limited by the operator's defaults, which is a question this column
-- answers directly).
--
-- source says what generated the spend: 'model' (assistant messages),
-- 'tool' (usage reported by tools and reconciliation deltas), 'compaction',
-- 'guardrail' (av-gust's screen — operator money even in a BYO-key session,
-- metered separately so overhead is visible and policy can differ).
--
-- cost_micros is Pi's own price-table estimate (USD millionths), kept beside
-- the exact token counts because a spend cap is denominated in money. It is
-- NOT a bill and must never be shown to a user as an amount owed; the token
-- columns are the meter. A model Pi has no price for records 0 here and the
-- tokens still say what happened.
--
-- No foreign key on owner_id: this is accounting. The money was spent even if
-- the account is later deleted, and an attribution ledger that self-shredded
-- on account deletion would not be one.

CREATE TABLE IF NOT EXISTS agent_usage (
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

-- The owner budget reads (owner_id, paid_by, source, recorded_at); the
-- instance ceiling reads (paid_by, recorded_at); the per-session ceiling and
-- the settle reconciliation read (session_id). Three indexes, one per query.
CREATE INDEX agent_usage_owner_period ON agent_usage(owner_id, recorded_at);
CREATE INDEX agent_usage_session ON agent_usage(session_id);
CREATE INDEX agent_usage_paid_period ON agent_usage(paid_by, recorded_at);

-- +goose Down
DROP TABLE IF EXISTS agent_usage;
