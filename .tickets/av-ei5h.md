---
id: av-ei5h
status: closed
deps: []
links: [av-fafu, av-7k7b]
created: 2026-09-18T15:22:01Z
type: feature
priority: 2
assignee: Max Omdal
---
# Serve gallery widgets on shared links (/s/:shareID/widget)

The render surface serves widgets only on the token-gated /w/:id route (ServeWidget) and shares only the artifact document (/s/:shareID -> ServeShare -> SourceBlobID). docs/widgets.md Known limits records this explicitly: shares serve the artifact only, there is no shared widget. Add a share-authorized widget render so a public link can embed the glanceable tile off-site.

## Design

New handler ServeShareWidget mirroring ServeShare: resolve GetAnonymousShareUnscoped + GetArtifactUnscoped (share row stays the auth, so a grant id 404s like a bad id; no new store accessor), 404 when WidgetBlobID is empty, then serveDoc with WidgetBlobID, widget=true, Claims{OwnerID: a.OwnerID, ViewerID: a.OwnerID} (owner state inlined, matching the shared artifact doc), and shareFrameAncestors framing (share policy / EMBED_ORIGINS default *, not /w app-only, since the point is off-site embedding). Read-only by construction: WIDGET short-circuits persistState and omits bridges; Permissions-Policy already denies camera/mic for widgets. CSP/asset manifest/origin decisions/no-store come free from serveDoc. Routes: RENDER_ORIGIN GET /s/{shareID}/widget plus APP_ORIGIN GET /s/{shareID}/widget 302 to the render origin beside serveShare. No migration, no token format change.

## Acceptance Criteria

GET RENDER_ORIGIN /s/:id/widget serves the widget doc with widget preamble (no capability bridges, camera/mic denied), owner state inlined, and share framing headers; 404 when the artifact has no widget; grant ids answer as nonexistent ids; app-origin /s/:id/widget redirects to the render origin; route walks updated (pageowner, pagecredential, csrf, hostdispatch dual-claim exemption, renderheaders rows); share panel surfaces the widget embed snippet; docs/widgets.md Known limits line removed.

