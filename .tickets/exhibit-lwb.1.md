---
id: exhibit-lwb.1
status: closed
deps: []
links: []
created: 2026-06-30T02:57:28Z
type: task
priority: 2
parent: exhibit-lwb
---
# Persist source URL/base on artifacts

Vendoring can't resolve relative refs without the original document base. URL ingest currently fetches req.URL (artifacts.go:104) then discards it. Add a source_url column to the artifacts schema (goose migration), capture req.URL at ingest, and expose it via the Store interface so the resolver can compute the base. Forward-looking column per PRD §4.4 'add columns now'.

## Acceptance Criteria

source_url persisted on URL-imported artifacts and readable through Store; nil/empty for paste/upload ingest; migration runs on fresh + existing DB.


