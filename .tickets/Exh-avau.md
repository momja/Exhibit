---
id: Exh-avau
status: open
deps: [epi-q0u2]
links: [av-ec0t]
created: 2026-07-10T06:17:35Z
type: epic
priority: 2
assignee: Max Omdal
tags: [build, deploy, static-export, epic]
---
# Static build export — deployable, read-only build of an Exhibit instance's artifact library

Add a build mode that turns a real, running Exhibit instance's artifacts into a self-contained static site: no server process, no API, no DB at request time — just files a deployer can host anywhere (GitHub Pages, S3, Netlify, etc.). First use case is a public demo instance so people can see what Exhibit does, using real example artifacts from an actual instance, before they self-host. The static build must reuse the same render logic the live service uses (render surface HTML composition, storage shim, CSP generation) rather than reimplementing it — it's an alternate *output* of that logic, not a parallel code path.

Blocked on [[epi-q0u2]]: the render/gallery HTML in `internal/api/gallery.go` is currently ~900 lines of inline Go string concatenation. Building a second output mode (static files) on top of that before it's split into templates would mean duplicating tangled string-concat code rather than reusing extracted template/asset logic. Do the template extraction first so the static build calls the same template functions the live handlers do.

Related to [[av-ec0t]] but not a duplicate: av-ec0t makes the *live* service serve an unauthenticated public gallery (still a running server, live DB/API, auth gating on mutating routes only). This epic instead produces an offline, file-based snapshot with no server at request time — a different mechanism for a similar "let people see this without full setup" goal. Linked for visibility since both touch public-facing read-only access.

## Design

- **Trigger:** a new subcommand on the existing Exhibit binary (not a separate tool), e.g. `exhibit build --manifest=<path>`. It only runs against a real instance's Store/Blob — it reads actual artifacts already in the DB, it does not fabricate or scaffold example content. This keeps "one binary, one process" (architecture.md §9) intact; the static build is a mode of the same program, not a new deployable.
- **Manifest:** build configuration only (output directory, base URL/path the static site will be served from, site title/tagline) — not a per-artifact allow-list. Scope is whole-library export: every artifact in the instance gets built. Narrower/curated selection is out of scope for v1 and can be layered on later if needed.
- **Per-artifact output:** reuse the render surface's document composition (storage shim injection, artifact body) as a static file per artifact, one HTML document per artifact ID. Two adaptations from the live path:
  - **CSP delivery:** the live render surface sets CSP via an HTTP response header (architecture.md §3.2). Static hosts generally can't set custom headers, so the static build bakes the same generated policy into a `<meta http-equiv="Content-Security-Policy">` tag in each document instead. Same policy content and allowlist-derived generation logic, different delivery mechanism.
  - **State:** the live shim inlines state at request time and bridges writes back to the API (PRD §5.3). A static build has no API to write through to, so state is a **read-only baked-in snapshot**: the shim still inlines each artifact's current state at build time so it renders correctly, but `setItem` becomes a no-op (or memory-only for the current page load). An artifact that depends on persisting new state won't retain it across reloads in the static build — this is an accepted, documented limitation of "read-only."
- **Gallery:** the static build also emits a browsable index (equivalent of the live gallery grid, with tag/collection groupings) linking to the per-artifact pages — not just bare permalinks. No live search (FTS5 is a runtime DB feature); browsing is via the index/tag/collection pages themselves.
- **Freshness model:** the static output is a point-in-time snapshot, matching the existing snapshot/refetch precedent (PRD §8.1) — re-running `exhibit build` regenerates it. No incremental sync, no versioning.

## Acceptance Criteria

- Running `exhibit build --manifest=<path>` against a real instance produces a self-contained static output directory containing every artifact currently in that instance's library.
- Each artifact's static page renders correctly with no server present: CSP is enforced via a baked `<meta>` tag generated from the artifact's `network_allowlist`, and its current state renders correctly via a read-only inlined snapshot (no write-through, no API calls attempted from the static page).
- A generated static index page lists/groups artifacts (grid + tags/collections) and links to each artifact's static page.
- The output directory is deployable as-is to a static host (verified against at least one real static host, e.g. GitHub Pages) with no additional server-side configuration.
- Docs describe the build command, manifest format, the read-only/no-sync limitation, and that re-running the build is how the static site is refreshed.

