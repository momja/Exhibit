---
id: exhibit-lwb.4
status: closed
deps: [exhibit-lwb.2]
links: []
created: 2026-06-30T02:57:39Z
type: task
priority: 2
parent: exhibit-lwb
---
# Inline CSS-embedded assets (recursive)

Relative URLs also hide in CSS — url() and @import inside <style> blocks, style= attributes, and fetched stylesheets. @import chains can reference further assets, so resolution must recurse (with the depth/count limits from the fetcher). Rewrite url() targets to data-URIs and either inline @import'd sheets or fold them in. CSS base is the stylesheet's own URL, not the document base, when the sheet was itself fetched from a subpath.

## Acceptance Criteria

url() and @import relatives in inline + fetched CSS are inlined; nested @import resolves against the correct per-sheet base; respects depth/size limits.


