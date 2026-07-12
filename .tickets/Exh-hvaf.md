---
id: Exh-hvaf
status: closed
deps: []
links: []
created: 2026-07-11T05:19:45Z
type: task
priority: 1
assignee: Max Omdal
parent: Exh-yvhp
---
# Exhibit tool extension for pi: save_artifact via the single write path

TypeScript pi extension registering save_artifact (create/update) tools that POST/PATCH the exhibit API using a scoped token from env. Output enters the library through the normal ingest path: scan, footprint, single write path. Conversation transcript persisted with the artifact (colophon provenance).

