---
name: verify
description: Build, launch, and drive Exhibit locally to verify changes end-to-end (app + render origins, API ingest, gallery pages).
---

# Verifying Exhibit changes

## Build & launch

```bash
sh scripts/build-assets.sh        # required once per checkout: populates internal/api/assets (go:embed)
go build -o /tmp/exhibit-server ./cmd/server
DATA_DIR=/tmp/exhibit-data APP_ORIGIN=http://localhost:8090 RENDER_ORIGIN=http://localhost:8091 \
  ADDR=:8090 RENDER_ADDR=:8091 /tmp/exhibit-server &
```

Defaults are :8080/:8081 — often already taken by a dev instance; use 8090/8091.
Auth is `Authorization: Bearer dev-token` (the AUTH_TOKEN default).

## Drive

- Ingest: `POST http://localhost:8090/api/artifacts` with `{"title","body","network_allowlist":[]}` → returns artifact id.
- Gallery detail page (host frame + sandboxed iframe): `http://localhost:8090/artifacts/<id>`.
- Raw render doc (shim + CSP inspection): `curl http://localhost:8091/a/<id>`.
- Update an artifact body without re-ingesting: `PATCH /api/artifacts/<id>` with `{"body": ...}`.

## Gotchas

- Browser-automation synthetic clicks do NOT reach the sandboxed cross-origin
  iframe (OOPIF), and coordinate clicks on the host page are unreliable under
  display scaling. Instead: make the test artifact fire its behavior from a
  `setTimeout`, and drive host-page buttons with `element.click()` via the
  JavaScript tool. Host-side `postMessage` traffic can be observed by
  installing a logging `message` listener right after navigation.
- Chrome blocks repeated downloads that carry no user gesture (automatic
  downloads protection) — the first gesture-less download per origin lands,
  later ones are silently dropped. Real user clicks propagate activation and
  are unaffected; don't mistake this for a bridge failure.
- `read_console_messages` sees only the app-origin page, not the opaque-origin
  iframe's console.
