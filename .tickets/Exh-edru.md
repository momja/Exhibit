---
id: Exh-edru
status: closed
deps: []
links: []
created: 2026-10-21T18:10:46Z
type: bug
priority: 2
assignee: Max Omdal
---
# Separate new-tag creation from existing-tag selection on edit page

The edit-page tag creator depends on selecting Create new tag in the existing-tag dropdown, even while the name and color fields are visible. Provide an independent creation control and explicit create-and-attach action.

## Acceptance Criteria

Existing tags attach through their own selector and button. New tags can be created and attached without any dropdown selection. Validation is scoped to the relevant action; tag changes refresh through htmx; desktop and mobile controls remain usable.


## Notes

**2026-10-21T18:15:29Z**

Implemented separate existing-tag and new-tag actions, with an explicit Create and add tag button and creation-specific validation. Removed the create option from the dropdown, retained htmx refreshes, and migrated tags.js to a native ES module. Browser checks confirmed creation with an empty dropdown, existing-tag attachment after refresh, blank/duplicate-name errors, preserved unsaved title edits, and layouts down to 320px. Screenshots: docs/screenshots/Exh-edru/01-tag-controls-desktop.png and 02-tag-controls-mobile.png. Validation so far: make assets; all 364 gallery JS tests; targeted Go tag-panel template/partial tests.

**2026-10-21T18:18:01Z**

The full internal/api run passed every test except an existing exact-markup assertion for the Tags panel opening element. Updated that assertion for the new artifact ID data attribute; the panel symmetry, tag-panel render, and tag partial checks pass together. No production changes were needed after browser verification.
