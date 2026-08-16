---
id: av-rclm
status: open
deps: []
links: []
created: 2026-08-09T22:39:13Z
type: task
priority: 2
assignee: Max Omdal
---
# Support OpenGraph tags for rich link previews

OpenGraph tags let a shared link render a preview snapshot of the content. For a public shared artifact, snapshot its glance/widget and use that as the OpenGraph view. If the artifact is private, use a default image displaying the Exhibit logo.

Share boundary (verified against current code): `ServeShare` (internal/render/render.go) checks `ExpiresAt` but ignores `Share.Public` — a private share currently serves the artifact document. The OpenGraph work must not widen that: artifact-derived metadata is served only for public, unexpired shares.

## Acceptance Criteria

- The required tags are enumerated (`og:title`, `og:description`, `og:image`, `og:type` at minimum), the image format (PNG) and dimensions (matching the widget tile) are specified, and the snapshot source is defined (the artifact's widget document, never the full artifact).
- Public, unexpired shares serve artifact-derived metadata; private, expired, and exact-boundary-time shares serve only generic Exhibit metadata.
- `ServeShare` respects `Share.Public`: a private share does not serve the artifact document, and no stored snapshot is served after expiry (the current gap is closed here, not merely worked around).
- `Cache-Control: no-store` is set on all metadata responses.
- Tests cover public, private, expired, and exact-boundary share states.

