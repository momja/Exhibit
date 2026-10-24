---
id: Exh-wi2y
status: closed
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

**2026-10-24T05:24:31Z**

User reported indefinite Loading your account. Browser inspection found missing Web Crypto on HTTP preview; HTTPS via tailnet-only Tailscale Serve port 8444 now proxies the app and old login/signup links redirect to configured HTTPS origin. Verified secureContext=true, crypto.subtle available, live signup form rendered, no console errors. Added 15-second SDK/script loading timeout with actionable error; two timeout tests and focused Go Clerk tests pass. This removes an observed HTTP compatibility problem; user must confirm their browser now loads/signs up. Preview renderer remains HTTP on distinct hostname; full artifact rendering from HTTPS preview needs a separate HTTPS render origin before claiming full end-to-end app validation.

**2026-10-24T05:32:49Z**

Enabled auth_passkey.used_for_sign_in on selected development instance through Clerk CLI after schema check and dry-run; re-pull confirms true, autofill/button enabled, satisfies_second_factor true. Production remains unconfigured. Added shared Clerk appearance using Exhibit tokens, matching typography/buttons/fields/panels and light/dark mode. Pinned currently used SDK/UI CDN versions and replaced structural CSS selectors with appearance settings. Browser confirmed Use passkey instead, exact brand colors, dark token colors, and no overflow at 390px. Refreshed screenshots. Build and Clerk loading timeout tests pass. Passkey enrollment requires user sign-up/sign-in and their own authenticator; not performed on their behalf.

**2026-10-24: Phosphor panel icons**

Added self-hosted Phosphor SVG masks via the existing icon build and Clerk appearance elements: account/security navigation, manage-account/sign-out/add-account actions, passkeys, arrows, edit, and password visibility. Retains SDK semantics/ARIA and provider brand icons. Browser verified signup masks render and password toggle switches input type, eye/eye-slash mask, and accessible label. Signed-in panel mappings inspected against Clerk source; no user created for testing. Refreshed mobile signup screenshot. Assets/server build and all 363 gallery tests pass.

**2026-10-24: Prove default self-hosted deployment**

Started the exact hosted-preview binary separately with env -i, no Clerk/OIDC/login configuration, and a fresh database on ports 8098/8099. It bootstraps the existing local admin flow. Added scripts/smoke-self-hosted.py: starts the provided binary with a clean environment/temp database and verifies local form, absence of Clerk bootstrap, wrong-password/anonymous rejection, successful local session/API access, and replay rejection after logout. Smoke test passes. Focused local/OIDC/Clerk auth and API tests pass uncached. Browser local login shows zero external resource requests and no Clerk global. Saved self-hosted login screenshot; corrected stale deployment documentation that claimed the default had no login.

**2026-10-24: Independent review round 1 fixes**

Codex review-agent confirmed four initial concerns and found hosted local-admin provisioning could undermine succession. Claude Code security-review confirmed the Compose dev-token bypass; lifecycle/email defects were below its security exploitability threshold but remain correctness fixes. Implemented hosted dev-token startup refusal; current verified-email lookup for grants and bounded profile refresh; provider-enforced user self-deletion disable; provider session revocation plus durable deletion cutoff including provider timestamps; hosted-only atomic admin succession/provisioning safeguards. Focused authentication/store/API tests pass, including restart/replay, clock skew, concurrency, and upstream failure. Separate self-host smoke remains the regression proof. Beginning independent second review on this fix snapshot.

**2026-10-24: Review round 2 recovery fix**

Codex confirmed the five prior fixes, then found a signed-in/locally-disabled user was trapped on the login error without a switch-account action. Added explicit retry and sign-out controls, with tests for denied access and transient network failure (no automatic signout). Added SDK multi-page revocation/ended-session timestamp coverage. Preparing final review snapshot; full Go suite still running, gallery 363 tests and vet passed before this two-test recovery addition.

**2026-10-24: Review convergence**

Final code snapshot 4f62110. Codex review-agent round 3: No findings (full branch, including recovery and migration fixture delta). Claude round 2: No findings on full security integration through 25b69b8; its T3 command was bound to main so it applied the method against the explicit worktree. To satisfy the literal command request, ran actual Claude Code CLI /security-review in Exhibit-clerk on 4f62110 with read-only plan permissions: No findings on the final recovery/fixture changes, prior security fixes accepted. Final validations: full auth and API suites pass; full store rerun passes after updating both old-release rewind fixtures for migration036; go vet passes; gallery365/365; built-binary clean-env selfhost and hosted dev-token refusal smoke passes. Hosted preview restarted with reviewed binary and HTTP200 signup. No live user created; signed-in recovery/provider permission enforcement tested with mocked provider/browser-controller tests, not by altering a real account in a browser. Production setup and direct Dashboard deletion cleanup remain operator responsibilities.
