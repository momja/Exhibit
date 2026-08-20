---
id: av-w75m
status: in_progress
deps: []
links: [av-lvxq]
created: 2026-08-20T03:02:56Z
type: bug
priority: 2
assignee: Max Omdal
tags: [infra, deployment]
---
# Nested node_modules ship in every Docker build context

`.dockerignore` lists `node_modules/`, and Docker matches that pattern only at the context root. The four npm workspaces under `web/` each keep their own, so roughly 101 MB of dependencies is packed and sent on every image build:

    44M  web/icons/node_modules
    28M  web/pwa-icons/node_modules
    28M  web/editor/node_modules
    960K web/htmx/node_modules

None of it is used. `scripts/build-assets.sh` runs `npm ci` inside the image's Node stage (line 26), so the host copies are replaced. The existing comment in .dockerignore already states the intent — 'installed and produced inside the image's Node stage, never sent from the host' — so this is the pattern failing to express a decision that was already made.

Local builds only pay disk and time. It gets worse with av-lvxq: `fly deploy` uploads the context to a remote builder, so the 101 MB goes over the network on every deploy.

## Design

1. `**/node_modules/` rather than `node_modules/`. Docker matches .dockerignore patterns against paths relative to the context root, so an unanchored directory name catches only the top level. Keep the bare pattern beside it; it costs nothing and states the common case.

2. While here, exclude what is definitely not a build input: .tickets/, docs/, design_files/, dev/, .github/. Nothing is embedded from any of them — the only go:embed directives outside internal/api's assets, templates and migrations is internal/agent's ext/exhibit.ts. Leave the root markdown and LICENSE alone: they are small, and a context missing its README surprises whoever looks next.

3. Do not exclude Go test files. They cost little and excluding them makes `go build ./...` inside the image differ from the one outside it.

## Acceptance Criteria

1. No node_modules directory appears in the build context, at any depth.
2. The image still builds and the built assets are the ones the Node stage produced.
3. .tickets, docs, design_files, dev and .github are absent from the context.

