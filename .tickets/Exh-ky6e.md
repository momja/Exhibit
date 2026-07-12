---
id: Exh-ky6e
status: closed
deps: []
links: []
created: 2026-07-11T05:19:45Z
type: task
priority: 1
assignee: Max Omdal
parent: Exh-yvhp
---
# Encrypted BYO API key storage + agent settings API/UI

user_settings-style table (agent_keys keyed by owner_id+provider) storing API keys encrypted with AES-256-GCM under a server secret (EXHIBIT_SECRET env or generated key file in data dir). API: PUT/GET(masked)/DELETE /api/agent/key. Settings UI section for entering provider + key; key never returned to page JS after entry (masked display only).

