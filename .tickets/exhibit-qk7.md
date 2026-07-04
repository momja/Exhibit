---
id: exhibit-qk7
status: open
deps: []
links: []
created: 2026-07-02T05:36:40Z
type: task
priority: 2
---
# Extract logo brand colors to shared constants and apply brand blue to the interface

The three brand colors currently live as raw hex literals buried inside the exhibitLogoSVG string in internal/api/logo.go: #23559e (blue), #fae317 (yellow), #de281d (red). Meanwhile the gallery interface uses an unrelated accent blue (#0070f3, with #005ed4 as its hover shade) that predates the logo. Goal: make the brand palette a single source of truth and align the UI with it.

Two parts:
1. Extract the three logo colors into named Go constants (e.g. brandBlue/brandYellow/brandRed in logo.go or a small colors.go) and have exhibitLogoSVG reference them so the artwork and the CSS draw from the same source. NOTE: the user quoted the blue as '24559e' but the actual value in logo.go is '#23559e' — #23559e is authoritative.
2. In the gallery interface, replace the current accent blue #0070f3 (16 occurrences) and its hover shade #005ed4 (2 occurrences) with the brand blue #23559e, choosing a suitable darker hover shade. These are spread across THREE separate inline <style> blocks (renderGalleryPage, renderDetailPage, renderEditPage in internal/api/gallery.go), so consider a shared color source (Go string interpolation, or a :root CSS custom property like --brand-blue emitted once) rather than pasting the hex 18 times.

## Design

Single source of truth for the palette. Option A: Go-level constants that both the SVG builder and the CSS string interpolation consume. Option B (complements A): emit a :root{--brand-blue:...} custom property in each page's <style> and reference var(--brand-blue), so the value is stated once per document. Pick whichever keeps gallery.go readable given it has 3 duplicated style blocks. Verify contrast: brand blue #23559e on white for links/buttons, and a hand-picked darker hover (e.g. ~#1a4076) since #23559e has no existing hover pair.

## Acceptance Criteria

1. logo.go exposes named constants for the three brand colors (blue #23559e, yellow #fae317, red #de281d) and exhibitLogoSVG renders identically using them (favicon + header logo still display correctly). 2. No occurrences of #0070f3 or #005ed4 remain in internal/api/gallery.go; interface accent/link/button color is brand blue #23559e with a working hover state. 3. go build ./... and go test ./... pass. 4. Gallery, detail, and edit pages all reflect the new accent color.


