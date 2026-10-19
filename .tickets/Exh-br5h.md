---
id: Exh-br5h
status: closed
deps: []
links: []
created: 2026-10-19T20:08:51Z
type: feature
priority: 2
assignee: Max Omdal
---
# Ask users questions in agent chat

Add an agent question tool with multiple choice and free-form answers, batched questions, in-chat interaction and safe session lifecycle handling. Encourage plain-language clarification, helpful suggestions and confirmation of inferred intent.

## Acceptance Criteria

Agents can ask one or several questions together; users can choose suggestions or write their own answers; answers resume the tool; stop/disconnect and invalid or stale answers are handled; prompts guide agents to clarify vague intent in everyday language.


## Notes

**2026-10-19T20:27:56Z**

Implemented ask_user with up to four grouped questions, optional single or multiple selections, custom answers and details, owner-scoped answer endpoints, server-rendered htmx cards, reconnect draft preservation, cancellation, idle expiry and saved history. Added plain-language clarification and assumption-confirmation guidance; background widget jobs omit the question tool. Validation: go test ./... passed; final focused Go and gallery script suites passed; targeted Go race checks passed. Verified desktop and mobile dark-mode UI, custom-answer switching, reconnect, submission and skip with the collaborative browser. Screenshots are in docs/screenshots/Exh-br5h/.

**2026-10-19T21:47:27Z**

Reworked the question UX after user feedback: removed the nested outer card and option boxes, generic heading, repeated instructions, tool-status chip and welcome banners. Questions now use plain radio/checkbox rows; Something else discloses a custom answer. Typing replaces a single selection, selecting a suggestion clears the custom answer, and all questions still submit together with Continue or Skip. Simplified empty-preview copy and agent guidance. Updated the design canvas, desktop/mobile screenshots and interaction tests. Validation: focused agent/API/mock tests including gallery scripts and real Pi question round trips passed; browser checks covered switching answers, reconnect draft retention, submission, phone dark mode and overflow.
