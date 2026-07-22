---
id: av-hic3
status: open
deps: []
links: []
created: 2026-07-22T04:16:21Z
type: bug
priority: 2
assignee: Max Omdal
---
# FTS search 500s on queries containing FTS5 syntax characters

ListArtifacts passes the raw user query + '*' straight into artifacts_fts MATCH, so a gallery search containing FTS5 metacharacters (< > " ( ) : - ^) returns a 500 instead of results — e.g. searching '<script>' errors with 'fts5: syntax error near "<"'. Fix by treating input as literal text: double-quote each whitespace-separated token (escaping inner quotes) before appending the prefix '*'. Surfaced while reviewing av-b6o9; pre-existing on main but more likely now that source text is searchable.

