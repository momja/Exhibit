---
id: av-jlp8
status: closed
deps: []
links: []
created: 2026-07-22T05:24:25Z
type: bug
priority: 2
assignee: Max Omdal
tags: [render, security, csp]
---
# Sandboxed iframe blocks form submission (allow-forms not set)

Browser console error: "Blocked form submission to '' because the form's frame is sandboxed and the 'allow-forms' permission is not set." Artifacts containing <form> elements cannot submit at all in the render iframe, since sandbox="allow-scripts" omits allow-forms.

## Design

Add allow-forms to the sandbox attribute on the render iframe (internal/api/templates/detail.tmpl and internal/api/agentui.go preview iframe). This must NOT be done in isolation: CSP's form-action directive does not inherit from default-src, so enabling allow-forms without adding an explicit form-action to buildCSP() (internal/render/render.go) would let artifacts submit forms to ANY origin, bypassing the connect-src allowlist entirely -- a bigger hole than the bug itself. Fix: extend buildCSP to emit form-action built from the same network allowlist, defaulting to form-action 'self' when the allowlist is empty (mirrors the existing connect-src 'none' default; same-page/empty-action form submits carry no network egress). Do NOT use the clipboard/downloads host-frame-bridge pattern here -- that pattern exists because those capabilities have no CSP knob to scope them. Forms already have one (form-action), so this should ride the existing scan/approve/allowlist/CSP model, not a new approval flow.

## Acceptance Criteria

- sandbox attribute includes allow-forms on both the gallery detail iframe and the agent preview iframe
- buildCSP emits form-action <allowlist origins> when the allowlist is non-empty, and form-action 'self' when empty
- form submission to an allowlisted origin succeeds; submission to a non-allowlisted origin is blocked by the browser (CSP), not silently allowed
- existing CSP/render tests updated to assert the new directive
- render_test.go comment block near allow-downloads is a good model for documenting the form-action reasoning inline

