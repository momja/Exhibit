---
id: av-i7hd
status: closed
deps: []
links: []
created: 2026-07-24T21:15:01Z
type: bug
priority: 1
assignee: Max Omdal
tags: [security, api, csp]
---
# Validate and normalize allowlisted network origins

The API accepts any string into an artifact's network allowlist and stores it verbatim. There is no validation and no normalization, so non-origin values reach the render CSP.

Observed on a live artifact (ab4fc00e, the ffmpeg video editor). Its generated CSP contained, in every directive:

  https://unpkg.com
  https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/esm/worker.js
  https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/esm/worker.js.     <- trailing dot
  https://exhibit.maxomdal.com/favicon.ico

Three distinct defects visible in one artifact: full URLs with paths stored as if they were origins, a duplicate that differs only by a trailing dot (from a sentence-terminating period in the source text), and near-duplicates of an origin that is already present.

Impact:
- Correctness/UX: the allowlist editor is supposed to present a short list of origins the user decides about. Path-bearing near-duplicates make the decision list unreadable and make 'one decision per (artifact, origin)' (architecture.md 3.3) false in practice, since the same origin can appear under several spellings.
- Security: a path-bearing source expression is path-matched by CSP, so entries silently mean something different from what the UI implies. Worse, an unnormalized entry can be broader than intended (e.g. a bare host, a scheme-less value, or a value with credentials/port variations) while the user believes they approved one specific origin.
- The write path is the single write path (PRD 4.1), which is exactly where this validation belongs.

No input sanitation exists today: POST /api/artifacts and PATCH /api/artifacts/:id take req.NetworkAllowlist and hand it to the store unchanged (internal/api/artifacts.go, allowlist := req.NetworkAllowlist), and SQLiteStore.ReplaceAllowedOrigins writes the strings as-is.

## Design

Add one normalization+validation function and call it at the single write path, so no other caller can bypass it.

Suggested shape (internal/store or a small internal/origin package, reused by the scanner):

  func NormalizeOrigin(s string) (string, error)

Rules:
- Parse with net/url. Require an absolute URL with a scheme of https (or http, for local/dev origins - decide and document).
- Reduce to the origin tuple: scheme + host + explicit non-default port. Drop path, query, fragment, userinfo.
- Lowercase scheme and host; strip a trailing dot from the host (unpkg.com. and unpkg.com are the same origin to CSP but different strings to us).
- Reject: empty, relative, whitespace, wildcards, CSP keywords ('self', 'none', 'unsafe-inline', data:, blob:) - the no-egress schemes are already unconditional in buildCSP (av-x01o) and must never be user-supplied allowlist rows.
- Deduplicate the resulting set, preserving order.

Wire-up:
- Apply in POST /api/artifacts and PATCH /api/artifacts/:id before the store call. Reject the request with 400 and name the offending value, rather than silently dropping it, so the edit page can surface it.
- Apply the same normalization to the ingest scanner's output so the footprint the user approves is spelled identically to what gets stored (a footprint entry that normalizes to an existing allow row should not read as a new, unapproved origin).
- Consider a defensive normalization inside ReplaceAllowedOrigins so the store invariant holds even for a future caller.

Data migration: existing rows are already dirty. Either a goose migration that normalizes and de-duplicates artifact_network_origins (collapsing rows that map to the same origin, keeping decision='block' over 'allow' on conflict to avoid silently widening a policy), or leave existing rows and normalize on next write - decide and note the choice in the ticket.

## Acceptance Criteria

- A NormalizeOrigin (or equivalent) function exists with unit tests covering: path-bearing URL, trailing-dot host, uppercase host, default vs explicit port, userinfo, scheme-less input, wildcard, CSP keyword, blob:/data:, duplicates.
- POST /api/artifacts and PATCH /api/artifacts/:id reject a non-origin allowlist value with 400 and an error naming the value; in-process API tests cover both routes.
- An allowlist round-trip (write then read) returns normalized, de-duplicated origins.
- The scanned footprint and stored allow rows use the same spelling, so an already-approved origin never re-appears as unapproved.
- Decision recorded (in the ticket or docs) on whether existing artifact_network_origins rows are migrated or normalized lazily; if migrated, the migration collapses duplicates without widening any policy.
- docs/security.md notes that allowlist entries are origins, validated at the single write path.


## Notes

**2026-08-04T19:02:21Z**

Existing-row decision: MIGRATED, not lazily normalized. internal/store/migration_origins.go registers a Go goose migration at version 23 — renumbered off the originally-picked 12 during a rebase onto main, which had since claimed 12 for the widget_blob_id repair (av-9pm8) — (SQL can't parse URLs) that rewrites artifact_network_origins once: each value goes through origin.NormalizeOrigin, rows that only carried path/query/fragment/userinfo noise collapse onto the origin they always effectively named, values with no origin in them at all (CSP keywords, relatives, garbage) are dropped, and when several rows collapse onto one origin block wins over allow — so the repair can narrow a policy but never widen one. Lazy normalization was rejected because 'next save' may never come: an artifact nobody edits again would keep dirty rows forever, and the store invariant the rest of the code now assumes would hold only for recently-touched artifacts. Rendering never crashed on the old rows (they were joined into the CSP as-is), but a value containing ';' could have injected a CSP directive — the migration plus the store-side normalization closes that.
