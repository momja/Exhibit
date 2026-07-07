---
id: av-b6o9
status: open
deps: []
links: []
created: 2026-07-06T23:12:04Z
type: task
priority: 2
assignee: Max Omdal
---
# FTS search indexes title only — spec promises source + tags too

PRD §8.2 and architecture §3.3 promise full-text search over artifact source + title + tag text, but artifacts_fts (001_initial.sql) indexes only title (id is UNINDEXED). Extend the FTS5 table/triggers to cover artifact source text and tag names so gallery search matches spec. Docs updated (av-ijew) to state title-only until this lands.

