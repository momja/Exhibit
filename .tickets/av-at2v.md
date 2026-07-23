---
id: av-at2v
status: closed
deps: []
links: []
created: 2026-07-23T03:32:42Z
type: feature
priority: 2
assignee: Max Omdal
---
# Styled 404 page with the swinging Exhibit frame

Every app-origin HTML 404 is currently a bare plain-text http.Error(w, "not found", 404). Three sites: internal/api/gallery.go:48 (detail — artifact missing), internal/api/gallery.go:76 (edit — artifact missing), and unknown paths, which fall through to chi's default "404 page not found" because no NotFound handler is registered on the app mux (internal/api/api.go setupRoutes). Replace them with a server-rendered 404 page that looks like Exhibit and offers a way out (back to gallery + search). JSON /api/* routes must keep returning their JSON errors — this is for HTML page routes only.

## Design

Design is complete: three artboards in the Paper "Scratchpad" file (https://app.paper.design/file/01KWQV7Y6G82HDZJ5CKPJTSJ7Y/1-0) — "Exhibit — 404 (Desktop)", "Exhibit — 404 (Mobile)", and "Exhibit — 404 · Motion spec (the knock)"; PNG exports at ~/Downloads/404_examples (machine-local).

Concept: the header logo is a picture frame hanging from a nail, so the 404 hero is that frame knocked askew, rocking on the nail and settling. The gallery ground (#f0f0f0) reads as a gallery wall; a museum wall label ("NOT ON VIEW / Untitled (Artifact Not Found)") sits under the frame; copy and the real gallery controls (primary button, secondary button, search input) sit right.

Shape: new internal/api/templates/notfound.tmpl + a renderNotFoundPage view model in gallery.go; new web/gallery/notfound.css added to the files array in web/gallery/build.mjs (page sheets are copied verbatim, no bundler). Reuse tokens.css/components.css and the existing header markup. The hero frame reuses the logo artwork in internal/api/logo.go — do not fork a second copy of that SVG; factor a sizable/rotatable variant if needed.

Motion: pivot is the nail, at transform-origin 50% 4.2% of the logo box. Negative angles swing the frame's bottom to the right (struck on the left). Peaks -15, +9.5, -6.2, +4, -2.5, +1.4, -0.6, 0 over 2.8s ease-in-out, .25s delay, once; replay on click/hover; prefers-reduced-motion: reduce disables the animation and leaves the frame at -3deg. Full CSS is on the motion-spec artboard.

## Acceptance Criteria

Unknown app-origin paths, /artifacts/{missing}, and /artifacts/{missing}/edit all render the styled 404 page with HTTP status 404 (status must not become 200). /api/* 404s still return their existing JSON/plain errors. Page matches the desktop and mobile artboards; frame rocks on load and settles; animation is disabled under prefers-reduced-motion. No second copy of the logo SVG. Go tests cover the status code + a marker from the new template on all three routes; assets build clean (scripts/build-assets.sh); verification screenshots (desktop + mobile) in the scratchpad, not committed.

