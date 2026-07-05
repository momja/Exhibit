---
id: exhibit-tww.2.1
status: closed
deps: []
links: []
created: 2026-07-01T05:09:42Z
type: task
priority: 2
parent: exhibit-tww.2
---
# Integrate Phosphor Icons into the gallery/app shell

Foundation for all tag pill controls (and future UI). Add Phosphor Icons to the app UI (served from the app origin, which is not CSP-restricted). Prefer self-hosting/embedding the icon assets over a CDN for durability, consistent with the 'it's just a file' ethos; a CDN <link> is an acceptable first cut if embedding is deferred. Establish the usage pattern (e.g. <i class="ph ph-pencil-simple"></i>) that later UI stories follow. Icons needed by this epic: pencil (edit), x (detach), plus (add), trash (delete), check (confirm).

## Acceptance Criteria

Phosphor icons render in the gallery; a documented, repeatable markup pattern exists for adding an icon; the icon source is embedded or clearly loaded from the app origin. No console/CSP errors.


