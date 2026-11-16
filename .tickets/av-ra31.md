---
id: av-ra31
status: closed
deps: [av-eu3v, av-4ac9]
links: []
created: 2026-07-09T06:04:34Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-ec0t
tags: [frontend, ui, public-mode, branding]
---
# Frontend: Instance hero/tagline component

Add a small hero section above the public gallery that renders the admin-configured PUBLIC_INSTANCE_NAME as a heading and PUBLIC_INSTANCE_DESCRIPTION as a subheading/paragraph. This component only appears on the unauthenticated public gallery view when both values are non-empty. It should be visually lightweight and match the existing inline CSS aesthetic (no external design system). When values are empty, the gallery layout is unchanged.

## Acceptance Criteria

1. Hero renders on the public gallery when PUBLIC_INSTANCE_NAME and/or PUBLIC_INSTANCE_DESCRIPTION are set. 2. Name is rendered as an H1 or prominent heading. 3. Description is rendered as a subtitle below the name. 4. Hero is absent when config values are empty. 5. Hero is not shown on authenticated views. 6. Styling is consistent with the existing gallery's inline CSS.


## Notes

**2026-11-16T07:14:43Z**

Public mode was retired in Exh-lmor rather than completed. The public demo gallery is a separate site built from artifact exports, not an anonymous library in the core service.
