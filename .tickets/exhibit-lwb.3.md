---
id: exhibit-lwb.3
status: open
deps: [exhibit-lwb.2]
links: []
created: 2026-06-30T02:57:36Z
type: task
priority: 2
parent: exhibit-lwb
---
# Inline HTML-referenced assets

Walk the parsed HTML (x/net/html, same as scanner) and replace relative asset references with inlined content: <img src>/<source>/srcset and <link rel=icon> as data-URIs; <script src> and <link rel=stylesheet> as inline <script>/<style>. Leave anchor hrefs and form actions alone. Skip assets that exceed limits or fail to fetch (record them as residual). Operates on the fetched body before Blob.put.

## Acceptance Criteria

img/script/stylesheet/favicon/srcset/<source> relatives inlined; anchors+forms untouched; over-limit/failed assets left as residual references with a record entry.


