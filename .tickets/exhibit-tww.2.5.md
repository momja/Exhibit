---
id: exhibit-tww.2.5
status: closed
deps: [exhibit-tww.1.4, exhibit-tww.2.1, exhibit-tww.2.2]
links: []
created: 2026-07-01T05:09:46Z
type: task
priority: 2
parent: exhibit-tww.2
---
# Add-tag '+' button + modal (select existing or create new)

A trailing Phosphor '+' button after each card's pill collection opens an add-tag modal. The modal FIRST shows a dropdown to select an existing tag (from ListTags) OR a 'create new' option; choosing create-new reveals name+color fields (same controls as the edit modal, tww.2.4). Confirm -> create the tag if new (POST /api/tags) then attach it (POST /api/artifacts/{id}/tags/{tagID}), or just attach if existing. On success the new pill appears on the card. Depends on hardened create/attach (tww.1.4) and pills+Phosphor.

## Acceptance Criteria

'+' opens the modal; selecting an existing tag attaches it and the pill appears; 'create new' with name+color creates then attaches in one flow; attaching a tag the artifact already has is a no-op/handled gracefully; modal validates empty name.


