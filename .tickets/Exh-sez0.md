---
id: Exh-sez0
status: closed
deps: []
links: []
created: 2026-11-02T15:59:54Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-qkl7
tags: [connectors, security, ui]
---
# Warn when a credential's format does not match its destination origin

In custom connection setup the user chooses the destination origin. A tool or a message can ask them to paste an OpenAI key with origin https://api-openai.co, and the relay will faithfully send it there. Agents can only request provider IDs, which is good, but a person can still be talked into it.

## Design

- In the trusted connect UI, recognize well-known key formats (ghp_/github_pat_ → api.github.com, sk-ant- → api.anthropic.com, sk- → api.openai.com, xox* → slack.com, and so on) and compare against the chosen origin.
- A mismatch blocks saving unless the user explicitly confirms, with copy naming the expected destination. Show the registrable domain of the chosen origin prominently in all cases.
- The check runs server-side too, so the API cannot bypass it without an explicit override flag.
- The secret never leaves the form for this check beyond the existing create request.

## Acceptance Criteria

Tests for each recognized format and a mismatch; browser test of the warning on desktop and phone; screenshots updated.


## Notes

**2026-11-16T07:35:39Z**

Implemented on feature/Exh-sez0/credential-origin-warning. Recognized formats (Anthropic, OpenAI, GitHub, Stripe, Slack, GitLab, Hugging Face, npm, Google) are compared against the chosen origin's registrable domain (publicsuffix) at the single create path in connectors.Service.Create, so the API cannot skip the check; a mismatch is a 409 credential_origin_mismatch carrying the credential label and both origins, and the same request with confirm_origin_mismatch succeeds. The connect form names the destination host as the origin is typed and renders the warning plus a 'Send to <host> anyway' confirmation in server-rendered markup. Basic credentials are exempt because they are not key-shaped. Reconnect was left alone: it cannot change the origin, which is the field this warning is about.
