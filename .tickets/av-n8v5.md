---
id: av-n8v5
status: open
deps: [av-wmp6, av-4ac9]
links: []
created: 2026-07-09T06:04:24Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-ec0t
tags: [frontend, ui, public-mode, chrome]
---
# Frontend: Adaptive UI chrome (auth-aware header/nav)

Make the global UI chrome (header, minimal footer) adapt based on authentication state. When unauthenticated in public mode: header shows the Exhibit logo + PUBLIC_INSTANCE_NAME (if set) + a 'Sign in' link. When authenticated: header shows the existing full navigation and controls. The footer, if present, should be minimal in public mode (optional single-line copyright or 'Powered by Exhibit'). Ensure no broken links appear for unauthenticated users.

## Acceptance Criteria

1. Unauthenticated public-mode header shows logo + instance name (or fallback to 'Exhibit') + 'Sign in' link. 2. Authenticated header is unchanged from current behavior. 3. No management links (settings, upload) appear in the public header. 4. Footer is minimal or absent in public mode; present but unobtrusive in authenticated mode. 5. All navigation links are valid for the user's auth state.


## Complexity

M
