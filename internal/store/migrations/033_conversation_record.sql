-- +goose Up
-- av-y7td: keep an agent conversation as Pi itself records it, tied to the
-- artifact version it left behind.
--
-- agent_transcripts held a dump of Pi's message list (migration 007), written
-- after each turn and read by nothing. That list is lossy — Pi's own
-- compaction drops the turns it summarized — and it is not something a
-- session can be started from. What Pi writes natively is its session file:
-- append-only JSONL holding every entry, compacted or not, and the one format
-- `pi --session` resumes from. So that is what is stored, as it is, in the
-- row that already exists for the conversation. No second table: a
-- conversation is one row per (artifact, session), and the file is its body.
--
-- version_seq is the artifact's head version when the conversation last
-- settled (migration 032): the code and saved data the conversation was last
-- working against, which is what resuming it can offer to return to. It is
-- set in the same statement that stores the file, so the pair cannot disagree.
--
-- title is the conversation's first prompt, shortened, so a list can tell
-- conversations apart without reading any of them.
--
-- Rows written before this migration keep their message dump and have no
-- session file — NULL, not '', so that "has a file" is a question the record
-- header answers (typeof) without reading a value that can be megabytes: they
-- can be read, and cannot be resumed, which is exactly what they always were.
-- version_seq 0 means "unknown" for them.
--
-- Version numbering: 8, 12 and 23 are Go migrations with no file here, and the
-- highest .sql is 032. A migration must sit above *every* version that can
-- already be in a ledger, from either source (technical_stack.md §3);
-- migration_order_test.go walks both rules.
ALTER TABLE agent_transcripts ADD COLUMN title TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_transcripts ADD COLUMN session_file TEXT;
ALTER TABLE agent_transcripts ADD COLUMN version_seq INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE agent_transcripts DROP COLUMN version_seq;
ALTER TABLE agent_transcripts DROP COLUMN session_file;
ALTER TABLE agent_transcripts DROP COLUMN title;
