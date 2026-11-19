---
id: Exh-geph
status: closed
deps: []
links: []
created: 2026-11-10T19:08:32Z
type: bug
priority: 2
assignee: Max Omdal
---
# Keep gallery card titles to one line with faded clipping

Keep gallery card metadata compact, remove multiline title height floors across breakpoints, and fade clipped title text.

## Acceptance Criteria

Titles stay on one line on desktop and mobile; long text fades at the edge; metadata remains compact and controls remain usable.


## Notes

**2026-11-10T19:12:39Z**

Implemented compact 48px desktop / 52px mobile metadata rows and single-line titles with a 16px trailing fade. Retained 44px mobile title/control targets and full-title tooltips. Corrected the existing readability test to visit Search instead of an empty Starred landing page. Validation: make assets; TestReadabilityBrowser across 320/375/390/430/1280, both themes and 100%/200% text; TestMobileTargetsBrowser. Updated docs and four desktop/phone screenshots.
