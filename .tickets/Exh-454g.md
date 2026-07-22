---
id: Exh-454g
status: closed
deps: []
links: []
created: 2026-07-13T05:59:31Z
type: bug
priority: 2
assignee: Max Omdal
---
# Cannot change model without resetting API-key

The `key-modal` component (`internal/api/agentui.go`) requires the API key
field to be cleared and re-entered before `saveKey()` will submit, even when
the user only wants to change the model (or provider). `saveKey()` refuses to
save whenever the secret input still holds the masked placeholder, with the
error "Delete the masked key below and enter a new one to replace it." The
backend (`putAgentKey` in `internal/api/agent.go`) also unconditionally
requires a non-empty `api_key`.

Expected: changing the model dropdown and clicking Save should succeed
without touching the API key.

