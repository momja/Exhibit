---
id: Exh-w3h0
status: in_progress
deps: []
links: []
created: 2026-10-19T19:47:55Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, agent]
---
# Validate agent prompt text and image attachments before subprocess writes

The service-wide request body limit is present, but agentPrompt has no separate text limit, image count or decoded-size limit, base64 validation, or supported MIME validation. Enforce explicit prompt and image limits before writing to the subprocess; reject invalid attachments with readable 400 responses. Split from av-4bzn.

