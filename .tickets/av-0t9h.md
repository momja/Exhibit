---
id: av-0t9h
status: open
deps: []
links: []
created: 2026-07-06T22:54:19Z
type: feature
priority: 2
assignee: Max Omdal
tags: [security, allowlist, csp]
---
# Allowlist path wildcards: allow path-suffix wildcard, reject DNS/host wildcards

Extend network_allowlist entries beyond exact origins to support wildcard matching on the URL path only — e.g. https://example.com/api/* — while explicitly rejecting any wildcard in the DNS/host portion (https://*.example.com) or scheme.

Today entries are stored as a plain JSON array of origins, seeded by the ingest scanner (internal/scanner/scanner.go, which only ever emits exact scheme+host origins) and joined verbatim into the CSP directives by buildCSP (internal/render/render.go:132). Nothing validates entries at the write path, so a hand-typed CSP host wildcard like https://*.example.com would pass through and the browser would honor it — silently widening the policy in a way the product never advertised. This ticket both adds the path-wildcard capability and closes that validation gap.

## Design

Enforcement stays the browser CSP — no custom matching engine (per architecture.md §4: the security boundary is browser-enforced policy). CSP source expressions natively support path scoping: a source ending in "/" is a path-prefix match, and a source with a full path matches that exact resource. CSP has NO "*" wildcard inside paths, so the user-facing "/*" suffix is UX sugar that buildCSP normalizes to the CSP trailing-"/" prefix form when emitting the header. No "*" may ever appear in the path portion of an emitted CSP source.

Accepted entry forms:
  - exact origin:              https://example.com          (unchanged, scanner output)
  - origin + exact path:       https://example.com/api/data.json
  - origin + path wildcard:    https://example.com/api/*    (emitted as https://example.com/api/)

Rejected entry forms (validation error at the API):
  - any "*" in host:           https://*.example.com, https://api.*.example.com
  - any "*" in scheme:         *://example.com
  - bare "*" or other CSP keywords

Validation lives at the single write path (internal/api/artifacts.go: create + PATCH network_allowlist, and the ingest approve step) so the UI, extension, or any future client cannot bypass it. The scanner is unchanged — it continues to emit exact origins; path scoping is a user-editing refinement in the allowlist editor. Path-bearing entries flow into all fetch directives buildCSP emits (connect-src/script-src/style-src/img-src/font-src), which CSP supports.

Known CSP limitation to document in the allowlist editor help: per CSP3, path components are ignored when matching a request that has been redirected — path scoping narrows the first hop only, host+scheme remain enforced throughout.

## Acceptance Criteria

- API accepts allowlist entries of the three forms above (exact origin, origin+path, origin+path/*) on artifact create, PATCH, and ingest approval.
- API rejects, with a clear 400 error naming the offending entry, any entry containing "*" in the scheme or host — including previously-passing hand-typed CSP host wildcards.
- buildCSP normalizes a trailing "/*" to the CSP path-prefix form (trailing "/"); emitted headers never contain "*" in a path.
- A rendered artifact can fetch https://example.com/api/x when https://example.com/api/* is allowlisted, and is CSP-blocked fetching https://example.com/other or https://evil.example.com.
- Allowlist editor UI accepts the new forms, surfaces validation errors, and documents the redirect caveat.
- Unit tests cover entry validation (accept/reject matrix) and buildCSP normalization; scanner output remains exact origins only.

