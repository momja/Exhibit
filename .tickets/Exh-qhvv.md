---
id: Exh-qhvv
status: in_progress
deps: []
links: []
created: 2026-11-15T02:03:46Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, art]
---
# Production hero paintings: day/dusk art and separate layers

Produce final day and dusk paintings (poppies, Mondrian grid on the CRT) at production resolution, delivered as: a full composite, separate sky / clouds / hill layers with clean alpha (sky extended upward for the descent), and a phone crop.

The mock's layers were cut by script from flat images (/tmp/exhibit-mock/layers, throwaway) and show halos; real layers should come from the source art. Deliver web-ready formats (WebP/AVIF) with sizes for responsive srcset.

## Acceptance Criteria

Day and dusk each have composite, three layers and phone crop; layers recombine with no seams or halos; file sizes reasonable for a login page.

