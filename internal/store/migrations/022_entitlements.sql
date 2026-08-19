-- +goose Up
-- Per-owner entitlements (av-2p8z): what an owner is allowed, as data an
-- admin can set.
--
-- Version numbering: 8 and 12 are occupied by Go migrations in
-- migration_repair.go that have no file here, and 019/020 are claimed by work
-- in flight on other branches even though no file for them exists in this
-- tree. Two prior outages came from two migrations sharing a version — read
-- migration_repair.go's header before adding another, and never renumber one
-- that has run.
--
-- These are columns on `users` rather than a table of their own, which is what
-- makes "deleting an account removes its entitlement" true by construction
-- rather than by a statement somebody has to remember to write. It also means
-- an owner with no `users` row — owner 1 on a single-user instance — has no
-- entitlement to read, which is the correct answer there: the instance's
-- default applies.
--
-- Nothing here says *why* an owner is on the plan they are on. There is no
-- payment state, no external system's schema, and no vendor anywhere in this
-- repo; whatever maintains these values on a commercial instance is an
-- ordinary authenticated API client, calling PATCH /api/admin/users/:id like
-- any other client of the single write path.

-- The plan label is for display and for grouping, and nothing reads a limit
-- out of it. Limits are stored per user (below) precisely so an instance can
-- give one person a larger allowance without inventing a plan for them, and
-- so that renaming a plan can never move anybody's ceiling.
ALTER TABLE users ADD COLUMN plan TEXT NOT NULL DEFAULT '';

-- The one limit anything reads today (av-10bw enforces it). Three states, and
-- the distinction between the last two is the whole design:
--
--   NULL  — this owner has no limit of their own; the instance default applies.
--   >= 0  — this owner's ceiling in bytes, whatever the default is.
--
-- NULL is *absence*, not ambiguity: it resolves to the default, which is a
-- limit, so it never resolves to unlimited on an instance that has limits
-- switched on. Unlimited is an instance-wide state (limits switched off), not
-- a per-owner value, so there is no sentinel here to mistake for a byte count.
--
-- The CHECK is what makes "a row that makes no sense" a case the resolver can
-- actually be trusted about: a negative ceiling cannot be written through the
-- API (which refuses it) or around it (which this refuses). The resolver still
-- validates, because a schema is not the only way a row arrives.
ALTER TABLE users ADD COLUMN storage_limit_bytes INTEGER
    CHECK (storage_limit_bytes IS NULL OR storage_limit_bytes >= 0);

-- An opaque string an operator's own system uses to recognize this account,
-- in the spirit of a ticket's --external-ref. It carries no semantics here:
-- nothing parses it, nothing joins on it, and no code path behaves differently
-- because of its value.
--
-- It lives here rather than in that external system because it is durable with
-- the account — the account survives that system being rebuilt, replaced, or
-- dropped entirely. Deliberately named apart from `external_id`, which is the
-- identity a provider issued and is what a *person* is known by; these two
-- must never be confused at a scan site.
ALTER TABLE users ADD COLUMN entitlement_ref TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN entitlement_ref;
ALTER TABLE users DROP COLUMN storage_limit_bytes;
ALTER TABLE users DROP COLUMN plan;
