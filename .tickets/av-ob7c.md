---
id: av-ob7c
status: open
deps: []
links: []
created: 2026-07-06T05:18:35Z
type: bug
priority: 1
assignee: Max Omdal
tags: [security, api, allowlist]
---
# refetchSource() auto-approves scanned network origins without user consent

When a user updates a URL-sourced artifact via "Update from source" (refetchSource() in internal/api/gallery.go, POST /api/artifacts/:id/refetch handled by refetchArtifact in internal/api/artifacts.go), the handler re-scans the fresh snapshot and writes the scan result directly into network_allowlist (artifacts.go:305: updates["network_allowlist"] = scanner.Scan(...)). Every origin found in the new content is auto-approved with no user review.

This violates the security model (spec §6.2 / architecture §5): the scan is supposed to be transparency only — origins must be shown to the user and explicitly approved before they enter the allowlist that generates the render CSP. On refetch, the newly fetched (untrusted, possibly changed) content silently gains network access to any origin it now references, and any origins the user previously revoked are silently re-added. The client-side confirm() only warns about the overwrite; it never shows the origins or offers per-origin approval.

## Acceptance Criteria

- Refetch does not silently expand or replace network_allowlist.
- After fetching, newly discovered origins are presented to the user for explicit approval (same footprint-approval flow as ingest) before being added.
- Origins the user previously removed/revoked are not re-added without consent; previously approved origins that are still present remain approved.
- Until the user decides, the artifact renders with the previously approved allowlist (or no network), never with unapproved origins in its CSP.

