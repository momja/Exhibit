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
