---
id: Exh-r6a0
status: in_progress
deps: []
links: []
created: 2026-10-29T15:21:02Z
type: feature
priority: 2
assignee: Max Omdal
---
# Unify Clerk account modal with storage and complete account deletion

Use one Clerk account button. Add storage usage in the account modal, expose Exhibit-owned account deletion in the profile experience, and delete the Clerk user through the backend SDK alongside Exhibit cleanup. Preserve last-administrator protection, failure recovery, and non-Clerk profile behavior.

## Acceptance Criteria

Clerk users have one account control; storage is visible in the modal; deletion removes Exhibit data and Clerk identity with accurate confirmation; non-Clerk login retains profile functionality; focused backend and browser checks pass.

