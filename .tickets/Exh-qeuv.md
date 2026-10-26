---
id: Exh-qeuv
status: in_progress
deps: [Exh-35gj]
links: [av-0k5q]
created: 2026-10-26T01:02:13Z
type: epic
priority: 2
assignee: Max Omdal
tags: [gallery, sharing, frontend]
---
# Gallery spaces: My library / Shared with me / Shared by me (B2 navigation)

Delivers av-0k5q's decided design (2026-10-25 note): directed shares exist (av-lrae, av-awr4), so a recipient has a second set of artifacts to browse. The library becomes three *spaces* partitioned by provenance — My library (owned), Shared with me (grants naming me), Shared by me (my artifacts with a grant or the public link). Tags and collections are organization, not provenance, and stay filters.

Navigation is B2: one spaces partial, laid out as a left rail on desktop and a bottom tab bar on phones (pointer/width media query, two CSS layouts, one markup). Collections and tags live in a filter sheet on phones (bottom sheet) and in the rail on desktop, with active filters shown as removable chips above the grid. The spaces nav renders only once a second space has content: a single-user library looks exactly as it does today.

Builds on epic Exh-35gj (share_links/share_grants split, share_grants_by_recipient index, EXISTS owner scoping), PRs #207 -> #208 -> #209. Mockups: options A-D and B1/B2 shown in-thread on 2026-10-25.

Out of scope: a 'waiting on you' badge (no data behind it), re-sharing, copying a shared artifact into your library (av-0k5q decision: a share never changes ownership).

