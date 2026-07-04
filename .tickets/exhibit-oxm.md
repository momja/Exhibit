---
id: exhibit-oxm
status: closed
deps: []
links: []
created: 2026-06-30T15:46:24Z
type: task
priority: 2
assignee: Max Omdal
---
# Integrate Exhibit logo as favicon and header logo

Integrate the Exhibit brand logo (design_files/exhibit_logo.svg, currently UNTRACKED) into the gallery UI in two places: (1) the browser favicon, and (2) the site header in place of / alongside the plain 'Artifact Viewer' text title.

VERIFIED REALITY (2026-07-01) — trust this over the older notes below: the ACTUALLY-SERVED gallery is internal/api/gallery.go -> renderGalleryPage() (a Go string builder), wired at internal/api/api.go galleryIndex ('/'). web/templates/gallery.templ is VESTIGIAL / not wired in — do not spend effort on it. There is NO /static file server in the code: web/static/shim.js is unused (the real storage shim is inlined in internal/render/render.go), so do NOT rely on a /static/ route.

Tasks:
- Source SVG: it is untracked, so read it from the main checkout at design_files/exhibit_logo.svg and inline its contents.
- Favicon: add a <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,..."> (inline data URI, URL-encoded or base64) to the <head> emitted by renderGalleryPage in gallery.go.
- Header logo: render the logo in the header emitted by renderGalleryPage (around the 'Artifact Viewer' wordmark), replacing or sitting beside it, with an accessible name (alt text / aria-label). Inline <svg> or an inline data-URI <img>.
- Preferred approach: inline the SVG to match the gallery's existing all-inline style (its CSS is already inlined) and pull complexity downward. Only add a go:embed static handler + route if clearly warranted (precedent: sqlite.go go:embed) — and wire the route yourself if so.
- No templ generate step is in use. The logo MUST appear in the served renderer (gallery.go), not the dead .templ.

## Acceptance Criteria

exhibit_logo.svg lives in served static assets; favicon shows the Exhibit logo in the browser tab; the gallery header displays the logo with accessible alt/aria text; logo renders in the actually-served gallery (not just one of the two renderers).


