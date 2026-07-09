---
id: av-ec0t
status: open
deps: []
links: []
created: 2026-07-09T05:42:23Z
type: epic
priority: 1
assignee: Max Omdal
tags: [ui, auth, public-mode, frontend, epic]
---
# Exhibit public instance mode — gallery-first adaptive UI

Enable Exhibit to serve a public audience from a self-hosted instance without turning the app into a marketing site. The app root becomes a read-only artifact gallery for anonymous visitors; authenticated users see the existing upload + management UI. Based on open-source precedents (Mastodon, BookStack, Karakeep), the instance itself becomes the destination, not a separate landing page inside the repo.

## Design

Pattern: Gallery-First Hybrid (Community/Gallery-First). The root route ('/') conditionally renders based on authentication state and instance configuration. Unauthenticated users see a clean public artifact grid with an optional admin-configurable instance name/tagline. Authenticated users see the existing upload form + full gallery + controls. No marketing pages, newsletter signups, pricing tables, or 'Features' tours live inside the application repo. If a separate marketing site is needed later, it lives in a distinct repository/deployment.

## Acceptance Criteria

1. Root route '/' renders a read-only artifact gallery when the user is unauthenticated. 2. Root route '/' renders the existing upload form + gallery when the user is authenticated. 3. Admin can configure PUBLIC_INSTANCE_NAME and PUBLIC_INSTANCE_DESCRIPTION (env/config); if set, these render as a small hero/tagline above the public gallery. 4. Individual artifact pages ('/artifact/:id') are publicly readable without authentication. 5. No marketing bloat (pricing, newsletter, about brochure) is added to the app repo. 6. Authentication gating is applied only to mutating routes (upload, settings, tag edit, delete). 7. UI chrome (nav, footer) adapts: authenticated = full controls; unauthenticated = minimal chrome + login link. 8. Existing self-hosted behavior remains unchanged when public mode is not explicitly enabled/configured.

