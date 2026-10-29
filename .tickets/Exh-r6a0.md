---
id: Exh-r6a0
status: closed
deps: []
links: []
created: 2026-10-29T15:21:02Z
type: feature
priority: 2
assignee: Max Omdal
---
# Unify account navigation with Clerk profile, Exhibit settings, and complete deletion

Use one Clerk account button with a Settings menu link. Keep Clerk Profile and Security intact. Redesign /profile as Settings at /settings with the old route retained as an alias, show storage there, and delete the Clerk user through the backend SDK alongside Exhibit cleanup. Preserve last-administrator protection, failure recovery, and non-Clerk profile behavior.

## Acceptance Criteria

Clerk users have one account control; storage and deletion are visible in Settings; deletion removes Exhibit data and Clerk identity with accurate confirmation; non-Clerk login retains profile functionality; focused backend and browser checks pass.


## Notes

**2026-10-29T15:47:46Z**

Implemented the revised split: one Clerk account menu links to Settings at /settings; /profile remains an alias. Clerk Profile and Security are unchanged. Storage and deletion live in Settings. Hosted deletion removes the Clerk user through the Go SDK after guarded local erasure, with durable retries and a permanent identity fence. Verified focused Go and browser checks, full store tests, go vet, asset build, and all 407 gallery JS tests. Browser checks stub the provider UI boundary; no real Clerk account was deleted. Added desktop/mobile screenshots and a self-contained Paper preview.
