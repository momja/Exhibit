---
id: av-rclm
status: in_progress
deps: []
links: []
created: 2026-08-09T22:39:13Z
type: task
priority: 2
assignee: Max Omdal
---
# Support OpenGraph tags for rich link previews

OpenGraph tags let a shared link render a preview snapshot of the content. For a public shared artifact, snapshot its glance/widget and use that as the OpenGraph view. If the artifact is private, use a default image displaying the Exhibit logo.

## Refreshed against the current share model (2026-10-19)

The original text predates av-lrae and av-8ipt. `shares.public` and `shares.expires_at` no longer exist: a share row with no recipient is the artifact's one anonymous link, a row naming an account is a grant, and a share lives until it is deleted. `GetAnonymousShareUnscoped` is already narrowed to the link, so `/s/:id` serves nothing for a grant id. The "private share serves the document" gap the ticket described is closed. The states to cover are now: **live link**, **grant id**, **revoked link**, and **never-issued id**. The last three must be indistinguishable.

## Design

- **Where the tags go.** `ServeShare` injects them at the very top of the share document's `<head>`, ahead of the render preamble and any `og:` tags the artifact wrote itself. They are only injected on the share route. `/a/:id`, `/w/:id` and version views are token-gated and never crawled.
- **Tags:** `og:type=website`, `og:site_name=Exhibit`, `og:title` (the artifact title), `og:description` (fixed copy, so no artifact text beyond the title), `og:url` (`RENDER_ORIGIN/s/:id`), `og:image` (`RENDER_ORIGIN/s/:id/og.png`), `og:image:type=image/png`, `og:image:width`, `og:image:height`, `og:image:alt`, `twitter:card=summary_large_image`. All values are HTML-escaped.
- **Image:** a PNG of the artifact's **shared widget document** (`/s/:id/widget`), never the full artifact. That route already serves the monogram default tile when there is no widget. The viewport is 264x132 CSS px, matching the 132px card tile at a typical card width, captured at device scale 4, giving 1056x528 (2:1, which is what `summary_large_image` expects).
- **Renderer:** headless Chromium over CDP via `chromedp`, as an **optional sidecar** (technical_stack.md §8). The main image stays slim. `OG_SNAPSHOT_CDP_URL` points at a DevTools endpoint (for example `chromedp/headless-shell`). When it is unset, or a snapshot fails, the image is the generic Exhibit card.
- **Network during a snapshot:** the snapshotting browser may fetch from `RENDER_ORIGIN` and nothing else. Every other request fails at the CDP `Fetch` layer. A server-side browser rendering tenant code must not become an SSRF and screenshot exfiltration channel into the operator's network, even for origins on the allowlist.
- **Cost:** snapshots are cached in memory per share id for a short TTL. Concurrent requests are collapsed, and only a bounded number of renders run at once, so holding a public link cannot become a way to drive Chromium. The cache is consulted only **after** the share row resolves, so a revoked link stops serving its image on the next request.
- **Misses:** a grant id, revoked link or never-issued id on `/s/:id` gets one generic HTML 404 carrying only Exhibit metadata (title "Exhibit", the brand image at `RENDER_ORIGIN/og.png`). The response is identical for all three. `/s/:id/og.png` misses answer the plain 404.
- **Out of scope (follow-up):** generic OG tags on app-origin pages (`/artifacts/:id`, login). Those templates do not know the absolute app origin, which `og:image` needs.

## Acceptance Criteria

- The required tags are enumerated (`og:title`, `og:description`, `og:image`, `og:type` at minimum), the image format (PNG) and dimensions (matching the widget tile) are specified, and the snapshot source is defined (the artifact's widget document, never the full artifact).
- A live anonymous link serves artifact-derived metadata. A grant id, a revoked link and a never-issued id serve only generic Exhibit metadata, identically.
- No snapshot is served for a share id that no longer resolves to a live link, even from cache.
- `Cache-Control: no-store` is set on every share-route metadata response: the document, the 404 page, and the image.
- Tests cover the live link, grant id, revoked, and never-issued states, plus the snapshot network restriction.
