---
id: av-qo0j
status: closed
deps: []
links: [av-5n1e]
created: 2026-07-25T00:11:48Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, gallery, ingest]
---
# Split ingest out of the gallery into a dedicated /new studio page

The gallery index is currently two pages stacked on one: a full ingest form (title, Paste/URL tabs, CodeMirror source editor, snapshot toggle, scan result, status) sits above the search box and the artifact grid, pushing the library — the reason the page exists — below the fold. Move ingest to its own page at /new and turn the gallery back into a pure library.

The header's "Agent" link is replaced by a primary "Add artifact" button pointing at /new. The agent is not dropped: it becomes the third creation route on the studio page, alongside Paste HTML and From URL, which is where creation belongs.

Design reviewed on the local paper canvas as frame `d-studio` (B's three-route body with A's header). Three directions were explored; this is the chosen merge.

## Design

## Gallery (internal/api/templates/gallery.tmpl, web/gallery/index.{css,js})

- Delete the `.upload` block entirely (the div#upload and everything in it: #title, .mode-tabs, #body textarea/CodeMirror mount, #url-input, #snapshot-row, #scan-result, .upload-row, #status).
- Header: replace `<a href="/agent">…Agent</a>` with a primary button-styled `<a class="btn" href="/new"><i class="ph ph-plus"></i> Add artifact</a>`.
- .search-row moves to the top of <main>; the grid follows. No other gallery behavior changes — eager client-side search filtering, tag pills, the add-tag/edit-tag modals and the capability cluster all stay exactly as they are.
- index.js: move every ingest handler (setMode, ingest, the CodeMirror mount, snapshot toggle wiring, scan-result rendering) to a new web/gallery/new.js. index.js keeps search, tags and modals. Drop the now-unused ingest CSS from index.css into a new new.css.
- notfound.tmpl:49 links to `/#upload` — retarget to `/new`. The `id="upload"` anchor comment in gallery.tmpl goes away with the block.

## Studio page (new)

- Route: GET /new on the app origin, server-rendered via html/template like every other gallery page. New template internal/api/templates/new.tmpl + handler/view model in internal/api/gallery.go. Register in the mux next to the existing gallery routes.
- Header follows the established sub-page pattern from detail.tmpl:17 — `<a href="/">← Gallery</a>` then `<h1>Add artifact</h1>`. Do NOT introduce a Library/Studio segmented nav; the back link plus page title is the app's existing idiom.
- Lede: h2 "Three ways in." + one muted line.
- Three route tiles in a row, each an icon tile + name + one-line description:
  1. Paste HTML  — selected by default, expands the panel below.
  2. From URL    — swaps the panel's source field for the URL input and reveals the snapshot toggle.
  3. Build with agent — a plain link to the existing /agent page. It does not expand a panel.
  The first two are the existing Paste/URL modes, promoted from .mode-tabs buttons to tiles. Keep the same underlying mode state so ingest() is unchanged in behavior.
- Panel below the tiles: Title (optional) field, Source (CodeMirror, same mount as today), snapshot toggle, #scan-result, #status, and a footer row with a muted note about origin approval, a Cancel link back to /, and the primary "Add to library" submit.
- Snapshot toggle stays URL-mode-only, exactly as today. It must remain hidden under Paste HTML: internal/api/artifacts.go:219 rejects `snapshot` without a `url` with 400 "snapshot requires a source url", because snapshot.NewFetcher (internal/snapshot/fetcher.go:129) requires an absolute http(s) base to resolve references against. Vendoring absolute-URL assets out of pasted HTML is a separate, larger change — out of scope here, do not attempt it.
- On successful ingest the page redirects to the new artifact's detail page rather than staying put; the approval-then-PATCH allowlist flow is unchanged (persist first, network-inert, then PATCH the approved set).

## Out of scope

- No changes to POST /api/artifacts or any other API route. This is a UI reorganization; the single write path is untouched.
- No snapshot-for-pasted-HTML.
- No Library/Studio global nav, no left rail, no card thumbnails/glyphs.

## Acceptance Criteria

- GET /new returns a server-rendered page with the three route tiles and a working ingest panel; the gallery index no longer contains any ingest markup (no #upload, #body, #url-input, #snapshot-row).
- The gallery header shows a primary "Add artifact" button linking to /new and no longer shows the Agent link.
- Pasting HTML at /new ingests exactly as it does today, including the footprint approval step and the allowlist PATCH, and lands on the new artifact's detail page.
- URL ingest at /new shows the snapshot toggle; Paste mode never shows it. Submitting Paste mode never sends `snapshot: true`.
- The "Build with agent" tile navigates to /agent.
- The 404 page's "Upload an artifact" button targets /new and resolves (no dead /#upload fragment).
- Existing gallery tests that assert on ingest markup in the index are moved/rewritten against /new rather than deleted; go test ./... passes and `cd web/gallery && npm run lint` (if the workspace lints) is clean.

