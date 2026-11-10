---
id: Exh-e736
status: closed
deps: []
links: []
created: 2026-11-10T22:56:59Z
type: bug
priority: 2
assignee: Max Omdal
---
# Align gallery card star with the single-line metadata row

After single-line gallery titles shipped, the star appears above the title and metadata buttons. Reproduce the offset and center the star with its siblings.


## Notes

**2026-11-10T22:58:49Z**

Confirmed regression: card-star retained align-self:flex-start after metadata changed to align-items:center. Added starred fixtures and a browser assertion comparing star/action vertical centers. Focused desktop check failed before the fix; removing the override passes the full TestReadabilityBrowser matrix (320/375/390/430/1280, both themes, 100%/200% text) in 4.139s. Assets built; desktop/phone captures verified locally. Review images remain outside the repository.
