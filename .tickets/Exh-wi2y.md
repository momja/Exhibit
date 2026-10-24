---
id: Exh-wi2y
status: in_progress
deps: []
links: []
created: 2026-10-24T04:57:03Z
type: feature
priority: 2
assignee: Max Omdal
---
# Add Clerk login for hosted Exhibit while preserving self-hosted authentication

Use selected Clerk application app_3L7s3OLDRhgo3j6lkYE3xOwfS19 for hosted login. No existing users to migrate. Preserve local-password and OIDC for self-hosters. Add visible auth controls, protect routes, retain owner scoping and test both modes.


## Notes

**2026-10-24T05:06:27Z**

Implemented hosted Clerk mode in feature/Exh-wi2y/hosted-clerk. Preserved self-hosted password/OIDC; disabled hosted default credentials; added cookie isolation check, login/signup/user controls, SDK session verification, ownership mapping, docs, and tests. Validation: Go auth/API/server tests passed, focused Clerk/auth regressions passed, go vet passed, gallery 361/361 tests passed with NODE_OPTIONS=--experimental-vm-modules, server builds. Clerk skills installed. CLI doctor reports not logged in/not linked/no keys. Await user CLI auth login on t3code as maxomdal before targeting selected app app_3L7s3OLDRhgo3j6lkYE3xOwfS19. Remaining: CLI init/link as appropriate and env pull, real app startup/browser auth verification/screenshots. No real Clerk users created; no environment files read.

**2026-10-24T05:14:42Z**

CLI authenticated and linked selected Exhibit application; env pull wrote development keys to ignored .env.local without inspecting contents. Doctor passes all auth/link/key checks (production instance not configured). Development app running at http://t3code:8096, isolated renderer hostname on port 8097. Verified live Clerk sign-in/sign-up controls and unauthenticated private API 401; captured desktop/mobile screenshots and fixed nested-card/overflow layout. Focused API tests and server rebuild pass. User must perform first sign-up to verify signed-in user menu; no test user created in the existing Clerk application.
