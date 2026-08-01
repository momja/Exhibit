---
id: av-kyhl
status: open
deps: []
links: []
created: 2026-08-01T18:49:54Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, gallery]
---
# Infinite scroll for the gallery grid, via htmx

The library index renders every card in one server render, capped at ListArtifacts(Limit: 100) — a number nothing in the UI communicates and nothing lets you page past. A library larger than that is silently truncated.

Replace it with htmx-driven infinite scroll: the grid renders a first page, and a sentinel at the end of the grid hx-gets the next page and appends it. This is the same fragment pattern the agent preview pane (av-6m3e) and the widget preview (av-fafu) already use — a /partials/* route rendering the SAME named template partial the full page render uses, so the card has exactly one definition.

Wanted on its own merits (the 100 cap is a real bug), and a prerequisite for bounding live widget frames (av-fafu follow-up), which needs a page/window notion to hang a budget on.

## Design

  - Extract the card loop into a named partial the page and the fragment both render (the cardWidget partial is already factored this way, the card around it is not).
  - GET /partials/gallery-grid?offset=N&q=... returns the next page of cards plus the next sentinel; an empty page returns no sentinel, which is what ends the scroll.
  - Sentinel uses hx-trigger='revealed', hx-swap='outerHTML' so the sentinel replaces itself with the next page + next sentinel.
  - Must compose with the existing eager search: search currently refetches the whole gallery and swaps .grid innerHTML (index.js). A new query has to reset the offset, not append to the previous result set.
  - ListArtifacts already takes Limit/Offset, so the store needs nothing.

## Acceptance Criteria

A library of 250 artifacts is fully reachable by scrolling. Search resets paging rather than appending. The card markup has one definition shared by the page render and the fragment route.

