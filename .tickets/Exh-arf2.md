---
id: Exh-arf2
status: closed
deps: []
links: []
created: 2026-10-30T03:03:45Z
type: task
priority: 2
assignee: Max Omdal
tags: [ui, components]
---
# Consolidate app header in the UI component library

Extract a shared header component for gallery, account settings (/profile), artifact editor, admin users, and new artifact pages. Preserve gallery dimensions and page-specific controls.

## Acceptance Criteria

All five pages use shared header markup and CSS. Gallery desktop/mobile dimensions remain the baseline. Search, navigation, auth-aware controls, editor actions and unsaved-changes protection still work. Update component documentation and screenshots.


## Notes

**2026-10-30T03:18:02Z**

Extracted typed appHeaderView and shared appHeader partial/CSS; migrated gallery, /profile, artifact editor, /admin/users and /new. Preserved rendered gallery dimensions (37px desktop/33px phone) and page-specific navigation, Clerk mount, filter/search and save bindings. Added browser layout coverage for all five pages at six widths, including long editor return labels and Filter text containment. Updated component docs and desktop/phone screenshots. Validation: make assets and Go build; focused Go page/auth tests; AppHeaderBrowser, EditNavigationBrowser, GalleryFilterBrowser and markup tests; all 407 gallery Node tests with --experimental-vm-modules. Full go test ./... run was stopped after the stale editor-return markup assertion was identified; its updated test passed in the focused rerun.

**2026-10-30T03:28:54Z**

Rebased onto current origin/main before PR creation. Preserved the newly merged unified accountControl on gallery and hosted Settings, including recovery links; /profile remains an alias of /settings. Updated Clerk markup assertion, removed obsolete account-settings header CSS, and refreshed all screenshots. Post-rebase focused Go/browser tests (including Clerk account flow) passed in 25.9s; all 407 Node tests passed.

**2026-10-30T03:45:57Z**

Completed header migration for the remaining agent and not-found pages and deleted all body>header compatibility rules and page-level header CSS. Agent key actions now use the shared button/label markup with a native feature listener; shared button styling replaces the agent duplicates. Header height now comes from the shared responsive token and is a minimum, allowing account-service errors to expand the bar. Hidden account recovery anchors remain hidden. Added seven-page/six-width layout coverage, API-key button interaction, and gallery/Settings recovery and failure layout regression cases. Focused Go/browser suite passed (16.3s), all 407 Node tests passed, and screenshots were refreshed.

**2026-10-30T04:08:08Z**

Migrated the remaining full-page templates: artifact viewer, local sign-in, and hosted sign-in/sign-up/sign-out. Every one of the ten full-document app templates now calls appHeader; a template audit prevents omissions. Per user preference, the viewer retains its floating behavior as the shared Floating variant: pill, popups, restore tab and announcements moved into components.tmpl; floating styles moved to components.css; a registered floating-header Stimulus controller manages interaction lifetime. Scoped listeners, timers, animation frames and ResizeObserver are disposed on disconnect, including pending tab animations. Auth pages use the normal gallery-sized header with branding and page title, without authenticated controls; local sign-in stays scriptless. Added real-browser auth layout and floating interaction/disconnect/reconnect checks. Focused Go/browser suite passed (74.7s); all 409 gallery Node tests passed; Go build passed. Viewer and local sign-in screenshots added.
